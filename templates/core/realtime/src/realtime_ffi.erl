-module(realtime_ffi).
-export([get_env/1, redis_publish/2, redis_subscribe/2, redis_configured/1]).

get_env(Key) ->
    case os:getenv(binary_to_list(Key)) of
        false -> {error, nil};
        Value -> {ok, list_to_binary(Value)}
    end.

%% --------------------------------------------------------------------------
%% Redis pub/sub (best-effort: failures are silent by design - pub/sub must
%% not take down the broker). Enabled only when REDIS_URL is set.
%% --------------------------------------------------------------------------

redis_parse_url(Url) ->
    %% redis://[user:password@]host:port - Url may be a binary or charlist.
    UrlList = case is_binary(Url) of true -> binary_to_list(Url); false -> Url end,
    Rest = string:trim(string:trim(UrlList, leading, "redis://"), leading, "redis:"),
    {UserInfo, HostPort} =
        case string:split(Rest, "@") of
            [HP] -> {"", HP};
            [UI, HP] -> {UI, HP}
        end,
    {Host, PortS} =
        case string:split(HostPort, ":") of
            [H, P] -> {H, P};
            [H] -> {H, "6379"}
        end,
    Password =
        case string:split(UserInfo, ":") of
            [_User, Pass] -> Pass;
            [_] -> ""
        end,
    {Host, list_to_integer(PortS), Password}.

redis_command(Sock, Args) ->
    Bin = [iolist_to_binary([[$*, integer_to_list(length(Args)), $\r, $\n] |
        [begin
             B = iolist_to_binary(A),
             [[$$, integer_to_list(byte_size(B)), $\r, $\n], B, <<"\r\n">>]
         end || A <- Args]])],
    gen_tcp:send(Sock, Bin).

redis_connect(Url) ->
    {Host, Port, Password} = redis_parse_url(Url),
    case gen_tcp:connect(Host, Port, [binary, {active, false}, {packet, raw}], 2000) of
        {ok, Sock} ->
            case Password of
                "" -> {ok, Sock};
                _ ->
                    _ = redis_command(Sock, [<<"AUTH">>, unicode:characters_to_binary(Password)]),
                    case gen_tcp:recv(Sock, 0, 2000) of
                        {ok, <<"+OK", _/binary>>} -> {ok, Sock};
                        {ok, _} -> gen_tcp:close(Sock), {error, auth_failed};
                        {error, _} = E -> gen_tcp:close(Sock), E
                    end
            end;
        {error, _} = E -> E
    end.

redis_configured(_) ->
    case os:getenv("REDIS_URL") of
        false -> 0;
        _ -> 1
    end.

redis_publish(_Channel, _Message) ->
    case os:getenv("REDIS_URL") of
        false -> ok;
        Url ->
            case redis_connect(Url) of
                {ok, Sock} ->
                    _ = redis_command(Sock, [<<"PUBLISH">>, iolist_to_binary(_Channel), iolist_to_binary(_Message)]),
                    _ = gen_tcp:recv(Sock, 0, 2000),
                    gen_tcp:close(Sock),
                    ok;
                _ -> ok
            end
    end.

%% Subscriber: forwards every published payload to Dest; reconnects forever so
%% a redis blip never permanently severs the pub/sub bridge. Backoff on errors
%% so a dead redis does not spin the CPU.
redis_subscribe(Channel, Dest) ->
    case os:getenv("REDIS_URL") of
        false -> ok;
        Url ->
            spawn(fun() -> redis_subscribe_loop(Url, Channel, Dest, 1000) end)
    end.

redis_subscribe_loop(Url, Channel, Dest, Backoff) ->
    case redis_connect(Url) of
        {ok, Sock} ->
            _ = redis_command(Sock, [<<"SUBSCRIBE">>, iolist_to_binary(Channel)]),
            case redis_read_messages(Sock, Dest) of
                ok -> timer:sleep(Backoff),
                     redis_subscribe_loop(Url, Channel, Dest, 1000);
                {error, _} -> timer:sleep(Backoff),
                     redis_subscribe_loop(Url, Channel, Dest, 1000)
            end;
        {error, _} ->
            timer:sleep(Backoff),
            redis_subscribe_loop(Url, Channel, Dest, min(Backoff * 2, 30000))
    end.

%% RESP: *3\r\n$7\r\nmessage\r\n$N\r\nchannel\r\n$M\r\npayload\r\n
redis_read_messages(Sock, Dest) ->
    case gen_tcp:recv(Sock, 0) of
        {ok, Data} -> redis_parse_payload(Data, Dest),
                      redis_read_messages(Sock, Dest);
        {error, _} = E -> gen_tcp:close(Sock), E
    end.

redis_parse_payload(<<>>, _Dest) -> ok;
redis_parse_payload(Data, Dest) ->
    %% drop the "*3\r\n" array header, then read the three bulk strings
    case binary:split(Data, <<"\r\n">>) of
        [_, Rest] ->
            case redis_next_element(Rest) of
                {<<"message">>, Rest2} ->
                    case redis_next_element(Rest2) of
                        {_Channel, Rest3} ->
                            case redis_next_element(Rest3) of
                                {Payload, Rest4} ->
                                    %% gleam subjects: send {Ref, Msg} to the pid so the
                                    %% actor's selector matches instead of discarding.
                                    case Dest of
                                        {subject, Pid, Ref} -> Pid ! {Ref, {redis_message, Payload}};
                                        _ -> Dest ! {redis_message, Payload}
                                    end,
                                    redis_parse_payload(Rest4, Dest);
                                _ -> ok
                            end;
                        _ -> ok
                    end;
                {_Any, Rest2} -> redis_parse_payload(Rest2, Dest);
                error -> ok
            end;
        [_Partial] -> ok
    end.

redis_next_element(<<$$, LenBin/binary>>) ->
    {Len, Rest0} = parse_int(LenBin),
    case Rest0 of
        <<"\r\n", Payload:Len/binary, "\r\n", Rest/binary>> -> {Payload, Rest};
        _ -> error
    end;
redis_next_element(_) -> error.

parse_int(Bin) ->
    {Int, Rest} = lists:splitwith(fun(C) -> C >= $0 andalso C =< $9 end, binary_to_list(Bin)),
    {list_to_integer(Int), list_to_binary(Rest)}.