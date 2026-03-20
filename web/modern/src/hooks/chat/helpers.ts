import { Message } from '@/lib/utils';
import { ResponseStreamSummary } from './types';

export const extractTextAndReasoningFromOutput = (output: any[] | undefined): ResponseStreamSummary => {
  if (!Array.isArray(output)) {
    return { text: null, reasoning: null };
  }

  const textParts: string[] = [];
  const reasoningParts: string[] = [];

  for (const item of output) {
    if (!item || typeof item !== 'object') {
      continue;
    }

    const itemType = String(item.type || '').toLowerCase();

    if (itemType === 'message') {
      const contentArray = Array.isArray(item.content) ? item.content : [];
      for (const contentEntry of contentArray) {
        if (!contentEntry || typeof contentEntry !== 'object') {
          continue;
        }
        const entryType = String(contentEntry.type || '').toLowerCase();
        const entryText = typeof contentEntry.text === 'string' ? contentEntry.text : '';
        if (entryType === 'output_text' && entryText) {
          textParts.push(entryText);
        }
        if ((entryType === 'reasoning' || entryType === 'summary_text') && entryText) {
          reasoningParts.push(entryText);
        }
      }
    }

    if (itemType === 'reasoning') {
      const summaryArray = Array.isArray(item.summary) ? item.summary : [];
      for (const summaryEntry of summaryArray) {
        if (!summaryEntry || typeof summaryEntry !== 'object') {
          continue;
        }
        const entryText = typeof summaryEntry.text === 'string' ? summaryEntry.text : '';
        if (entryText) {
          reasoningParts.push(entryText);
        }
      }
    }
  }

  return {
    text: textParts.length > 0 ? textParts.join('') : null,
    reasoning: reasoningParts.length > 0 ? reasoningParts.join('\n') : null,
  };
};

export const convertMessageToResponseInput = (message: Message): Record<string, any> | null => {
  if (!message || message.role === 'error') {
    return null;
  }

  const role = message.role === 'assistant' ? 'assistant' : message.role === 'user' ? 'user' : message.role;

  if (role === 'system') {
    return null;
  }

  const textType = role === 'assistant' ? 'output_text' : 'input_text';
  const contentParts: any[] = [];

  const appendText = (text: string | undefined) => {
    if (!text) {
      return;
    }
    const normalized = String(text);
    if (normalized.trim().length === 0) {
      return;
    }
    contentParts.push({ type: textType, text: normalized });
  };

  const appendImage = (raw: any) => {
    if (role !== 'user' || !raw) {
      return;
    }

    if (typeof raw === 'string') {
      contentParts.push({ type: 'input_image', image_url: raw });
      return;
    }

    if (typeof raw === 'object') {
      const url = typeof raw.url === 'string' ? raw.url : typeof raw.image_url === 'string' ? raw.image_url : '';
      if (!url) {
        return;
      }
      const part: Record<string, any> = { type: 'input_image', image_url: url };
      const detail = typeof raw.detail === 'string' ? raw.detail : undefined;
      if (detail && detail.trim().length > 0) {
        part.detail = detail.trim();
      }
      contentParts.push(part);
    }
  };

  if (typeof message.content === 'string') {
    appendText(message.content);
  } else if (Array.isArray(message.content)) {
    for (const entry of message.content) {
      if (!entry) {
        continue;
      }
      if (typeof entry === 'string') {
        appendText(entry);
        continue;
      }

      const entryType = typeof entry.type === 'string' ? entry.type.toLowerCase() : '';

      if (entryType === 'text' || entryType === 'input_text') {
        appendText(typeof entry.text === 'string' ? entry.text : undefined);
        continue;
      }

      if (entryType === 'image_url' || entryType === 'input_image') {
        appendImage(entry.image_url ?? entry);
        continue;
      }

      if (typeof entry.text === 'string') {
        appendText(entry.text);
      }
    }
  }

  if (role === 'assistant' && typeof message.reasoning_content === 'string') {
    const trimmed = message.reasoning_content.trim();
    if (trimmed.length > 0) {
      contentParts.push({ type: 'reasoning', text: trimmed });
    }
  }

  if (contentParts.length === 0) {
    return null;
  }

  return {
    role,
    content: contentParts,
  };
};

export const buildResponseInputFromMessages = (messages: Message[]): any[] => {
  return messages.map(convertMessageToResponseInput).filter((entry): entry is Record<string, any> => entry !== null);
};
