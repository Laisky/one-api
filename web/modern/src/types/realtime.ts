export type EventDirection = 'in' | 'out';
export type EventTransport = 'ws' | 'sse';
export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected';

export interface EventLogEntry {
  id: string;
  direction: EventDirection;
  type: string;
  payload: unknown;
  timestamp: Date;
  transport: EventTransport;
}

export type AddEventInput = Omit<EventLogEntry, 'id' | 'timestamp'>;
