export interface SessionUser {
  id: string;
  email: string;
}

export interface HealthResponse {
  status: string;
  db: string;
  cache: string;
}

export interface BroadcastMessage {
  message: string;
  at: string;
}
