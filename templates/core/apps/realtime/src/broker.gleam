import gleam/erlang/process
import gleam/list
import gleam/otp/actor

/// In-memory pub/sub used by SSE and WebSocket clients, fanned out to Redis
/// pub/sub when REDIS_URL is set (horizontal scaling: other API/WS nodes
/// subscribe on the same channel and get every message).
///
/// Local clients always get messages in-memory (works even while Redis is
/// down); the Redis loopback of our own publishes is deduped via `recent`.

pub type Broker = actor.Started(process.Subject(BrokerMsg))
pub type State = #(List(process.Subject(String)), List(String))

pub type BrokerMsg {
  Register(process.Subject(String))
  Broadcast(String)
  RedisMessage(String)
}

@external(erlang, "realtime_ffi", "redis_subscribe")
fn redis_subscribe(channel: BitArray, subject: process.Subject(BrokerMsg)) -> Nil

@external(erlang, "realtime_ffi", "redis_publish")
fn redis_publish(channel: BitArray, message: String) -> Nil

fn fanout(state: State, message: String) -> State {
  let #(subjects, recent) = state
  list.each(subjects, fn(subject) { process.send(subject, message) })
  #(subjects, recent)
}

fn remember(state: State, message: String) -> State {
  let #(subjects, recent) = state
  #(subjects, list.take([message, ..recent], 64))
}

pub fn new() -> Broker {
  let assert Ok(broker) =
    actor.new_with_initialiser(1000, fn(subject) {
      redis_subscribe(<<"events">>, subject)
      Ok(
        actor.initialised(#([], []))
        |> actor.returning(subject),
      )
    })
    |> actor.on_message(fn(state, msg) {
      case msg {
        Register(subject) -> {
          let #(subjects, recent) = state
          actor.continue(#([subject, ..subjects], recent))
        }
        Broadcast(message) -> {
          redis_publish(<<"events">>, message)
          state |> fanout(message) |> remember(message) |> actor.continue
        }
        // own publish echoing back from redis - already fanned out locally.
        // (collision risk if a remote node broadcasts the exact same string
        // within the last 64 messages - fine for this template's traffic)
        RedisMessage(message) ->
          case list.contains(state.1, message) {
            True -> actor.continue(state)
            False -> state |> fanout(message) |> actor.continue
          }
      }
    })
    |> actor.start

  broker
}

pub fn register(broker: Broker, subject: process.Subject(String)) {
  actor.send(broker.data, Register(subject))
}

pub fn broadcast(broker: Broker, message: String) {
  actor.send(broker.data, Broadcast(message))
}