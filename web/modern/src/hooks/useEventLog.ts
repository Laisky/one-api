import { useCallback, useState } from 'react';

import type { AddEventInput, EventLogEntry } from '@/types/realtime';

export interface UseEventLogReturn {
  events: EventLogEntry[];
  expandedEvents: Set<string>;
  addEvent: (entry: AddEventInput) => void;
  toggleExpand: (id: string) => void;
  clearEvents: () => void;
}

export function useEventLog(): UseEventLogReturn {
  const [events, setEvents] = useState<EventLogEntry[]>([]);
  const [expandedEvents, setExpandedEvents] = useState<Set<string>>(() => new Set());

  const addEvent = useCallback((entry: AddEventInput) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
    const next: EventLogEntry = {
      ...entry,
      id,
      timestamp: new Date(),
    };
    setEvents((prev) => [...prev, next]);
  }, []);

  const toggleExpand = useCallback((id: string) => {
    setExpandedEvents((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const clearEvents = useCallback(() => {
    setEvents([]);
    setExpandedEvents(new Set());
  }, []);

  return {
    events,
    expandedEvents,
    addEvent,
    toggleExpand,
    clearEvents,
  };
}
