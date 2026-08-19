import gleam/erlang/process
import gleam/bit_array
import gleam/bytes_tree
import gleam/dict
import gleam/dynamic/decode
import gleam/http/request
import gleam/http/response
import gleam/json
import gleam/otp/actor
import gleam/result
import gleam/int
import gleam/option.{Some}
import gleam/string
import gleam/string_tree
import mist
import broker

@external(erlang, "realtime_ffi", "get_env")
fn get_env(key: String) -> Result(String, Nil)

fn env_or(key: String, default: String) -> String {
  get_env(key) |> result.unwrap(default)
}

fn text_resp(status: Int, body: String) -> response.Response(mist.ResponseData) {
  response.new(status)
  |> response.set_body(
    mist.Bytes(bytes_tree.new() |> bytes_tree.append_string(body)),
  )
}

fn json_resp(json: json.Json) -> response.Response(mist.ResponseData) {
  response.new(200)
  |> response.set_header("content-type", "application/json")
  |> response.set_body(
    mist.Bytes(
      bytes_tree.new()
      |> bytes_tree.append_string(json.to_string(json)),
    ),
  )
}

fn message_of(body: BitArray) -> String {
  case bit_array.to_string(body) {
    Ok(body) -> {
      case json.parse(body, using: decode.dict(decode.string, decode.string)) {
        Ok(dict) -> dict.get(dict, "message") |> result.unwrap("")
        Error(_) -> body
      }
    }
    Error(_) -> ""
  }
}

fn sse_handler(broker: broker.Broker, req: request.Request(mist.Connection)) -> response.Response(mist.ResponseData) {
  mist.server_sent_events(
    req,
    response.new(200),
    init: fn(subject) {
      broker.register(broker, subject)
      []
    },
    loop: fn(state, message, conn) {
      let _ = mist.send_event(conn, mist.event(string_tree.from_string(message)))
      actor.continue(state)
    },
  )
}

fn ws_handler(broker: broker.Broker, req: request.Request(mist.Connection)) -> response.Response(mist.ResponseData) {
  mist.websocket(
    req,
    handler: fn(state, message, conn) {
      case message {
        mist.Text(text) -> {
          let _ = mist.send_text_frame(conn, text)
          broker.broadcast(broker, "realtime: " <> text)
          mist.continue(state)
        }
        mist.Custom(msg) -> {
          let _ = mist.send_text_frame(conn, msg)
          mist.continue(state)
        }
        _ -> mist.stop()
      }
    },
    on_init: fn(_conn) {
      let subject = process.new_subject()
      broker.register(broker, subject)
      #([], Some(process.new_selector() |> process.select(subject)))
    },
    on_close: fn(_state) { Nil },
  )
}

fn broadcast_handler(broker: broker.Broker, secret: String, req: request.Request(mist.Connection)) -> response.Response(mist.ResponseData) {
  let header_ok =
    request.get_header(req, "x-backend-secret")
    |> result.map(fn(v) { v == secret })
    |> result.unwrap(False)

  case header_ok {
    False -> text_resp(401, "unauthorized")
    True -> {
      case mist.read_body(req, max_body_limit: 65536) {
        Ok(read_req) -> {
          let message = message_of(read_req.body)
          broker.broadcast(broker, "realtime: " <> message)
          json_resp(json.object([#("ok", json.bool(True))]))
        }
        Error(_) -> text_resp(400, "bad request")
      }
    }
  }
}

fn handler(broker: broker.Broker, secret: String) -> fn(request.Request(mist.Connection)) -> response.Response(mist.ResponseData) {
  fn(req: request.Request(mist.Connection)) {
    case req.path {
      "/events" -> sse_handler(broker, req)
      "/ws" -> ws_handler(broker, req)
      "/broadcast" -> broadcast_handler(broker, secret, req)
      "/health" -> text_resp(200, "ok")
      _ -> text_resp(404, "not found")
    }
  }
}

pub fn main() {
  let broker = broker.new()
  let port = env_or("PORT", "8001") |> string.trim |> int.parse |> result.unwrap(8001)
  let secret = env_or("BACKEND_SECRET", "change-me")

  let assert Ok(_) =
    mist.new(handler(broker, secret))
    |> mist.port(port)
    |> mist.bind("0.0.0.0")
    |> mist.start

  process.sleep_forever()
}