import gleam/erlang/process
import gleam/list
import gleam/otp/actor

/// In-memory pub/sub used by SSE and WebSocket clients, fanned out to Redis
/// pub/sub when REDIS_URL is set (horizontal scaling: other API/WS nodes
/// subscribe on the same channel and get every message).

pub type Broker = actor.Started(process.Subject(BrokerMsg))
pub type State = List(process.Subject(String))

pub type BrokerMsg {
  Register(process.Subject(String))
  Broadcast(String)
  RedisMessage(String)
}

@external(erlang, "realtime_ffi", "redis_subscribe")
fn redis_subscribe(channel: BitArray, subject: process.Subject(BrokerMsg)) -> Nil

@external(erlang, "realtime_ffi", "redis_publish")
fn redis_publish(channel: BitArray, message: String) -> Nil

@external(erlang, "realtime_ffi", "redis_configured")
fn redis_configured(_: Int) -> Int

pub fn new() -> Broker {
  let assert Ok(broker) =
    actor.new_with_initialiser(1000, fn(subject) {
      redis_subscribe(<<"events">>, subject)
      Ok(
        actor.initialised([])
        |> actor.returning(subject),
      )
    })
    |> actor.on_message(fn(state, msg) {
      case msg {
        Register(subject) -> actor.continue([subject, ..state])
        Broadcast(message) -> {
          // with redis on, our own publish loops back via the subscriber and
          // fans out once (identical to any remote node) - avoids dupes
          case redis_configured(0) {
            1 -> redis_publish(<<"events">>, message)
            _ -> list.each(state, fn(subject) { process.send(subject, message) })
          }
          actor.continue(state)
        }
        // redis.subscribe -> pub/sub fanout from other nodes; never republished
        RedisMessage(message) -> {
          list.each(state, fn(subject) { process.send(subject, message) })
          actor.continue(state)
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