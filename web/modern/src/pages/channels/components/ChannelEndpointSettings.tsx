import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormControl, FormField, FormItem } from "@/components/ui/form";
import { Info } from "lucide-react";
import { useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import type { ChannelForm, EndpointInfo } from "../schemas";
import { LabelWithHelp } from "./LabelWithHelp";

interface ChannelEndpointSettingsProps {
  form: UseFormReturn<ChannelForm>;
  allEndpoints: EndpointInfo[];
  defaultEndpoints: string[];
  tr: (
    key: string,
    defaultValue: string,
    options?: Record<string, unknown>
  ) => string;
}

// Endpoint documentation with descriptions and curl examples
const ENDPOINT_DOCS: Record<
  string,
  { description: string; curlExample: string }
> = {
  chat_completions: {
    description:
      "The Chat Completions API creates model responses for chat-based conversations. Send a list of messages and receive a model-generated reply. Supports both streaming and non-streaming modes.",
    curlExample: `curl https://oneapi.laisky.com/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello!"}
    ]
  }'`,
  },
  completions: {
    description:
      "The legacy Completions API generates text completions for a given prompt. This is the original text generation API, now largely superseded by Chat Completions.",
    curlExample: `curl https://oneapi.laisky.com/v1/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "gpt-3.5-turbo-instruct",
    "prompt": "Once upon a time",
    "max_tokens": 100
  }'`,
  },
  embeddings: {
    description:
      "The Embeddings API converts text into numerical vector representations. These vectors can be used for semantic search, clustering, and similarity comparisons.",
    curlExample: `curl https://oneapi.laisky.com/v1/embeddings \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "text-embedding-3-small",
    "input": "The quick brown fox jumps over the lazy dog"
  }'`,
  },
  moderations: {
    description:
      "The Moderations API checks whether text violates content policies. It returns categories and scores indicating the likelihood of policy violations.",
    curlExample: `curl https://oneapi.laisky.com/v1/moderations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "input": "I want to hurt someone"
  }'`,
  },
  images_generations: {
    description:
      "The Image Generation API creates images from text descriptions. Specify a prompt and receive one or more generated images in various formats and sizes.",
    curlExample: `curl https://oneapi.laisky.com/v1/images/generations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "dall-e-3",
    "prompt": "A serene mountain landscape at sunset",
    "n": 1,
    "size": "1024x1024"
  }'`,
  },
  images_edits: {
    description:
      "The Image Edits API modifies existing images based on text instructions. Upload an image with a mask and prompt to generate edited versions.",
    curlExample: `curl https://oneapi.laisky.com/v1/images/edits \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -F image="@original.png" \\
  -F mask="@mask.png" \\
  -F prompt="Add a rainbow in the sky" \\
  -F n=1 \\
  -F size="1024x1024"`,
  },
  audio_speech: {
    description:
      "The Text-to-Speech API converts text into spoken audio. Choose from multiple voices and output formats to generate natural-sounding speech.",
    curlExample: `curl https://oneapi.laisky.com/v1/audio/speech \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "tts-1",
    "input": "Hello, how can I help you today?",
    "voice": "alloy"
  }' --output speech.mp3`,
  },
  audio_transcription: {
    description:
      "The Audio Transcription API converts spoken audio into text. Upload an audio file to receive a transcript in the specified language.",
    curlExample: `curl https://oneapi.laisky.com/v1/audio/transcriptions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -F file="@audio.mp3" \\
  -F model="whisper-1"`,
  },
  audio_translation: {
    description:
      "The Audio Translation API translates spoken audio into English text. Upload an audio file in any supported language to receive an English transcript.",
    curlExample: `curl https://oneapi.laisky.com/v1/audio/translations \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -F file="@french_audio.mp3" \\
  -F model="whisper-1"`,
  },
  rerank: {
    description:
      "The Rerank API reorders a list of documents based on their relevance to a query. Useful for improving search results and retrieval-augmented generation (RAG) systems.",
    curlExample: `curl https://oneapi.laisky.com/v1/rerank \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "rerank-english-v2.0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of AI.",
      "The weather is nice today.",
      "Deep learning uses neural networks."
    ]
  }'`,
  },
  response_api: {
    description:
      "The Response API is OpenAI's newer stateful API for multi-turn conversations. It manages conversation state server-side and supports advanced features like tools and structured output.",
    curlExample: `curl https://oneapi.laisky.com/v1/responses \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "gpt-4",
    "input": "What is the capital of France?"
  }'`,
  },
  claude_messages: {
    description:
      "The Claude Messages API is Anthropic's native format for interacting with Claude models. One-API converts this format to work with any supported backend.",
    curlExample: `curl https://oneapi.laisky.com/v1/messages \\
  -H "Content-Type: application/json" \\
  -H "x-api-key: YOUR_API_KEY" \\
  -H "anthropic-version: 2023-06-01" \\
  -d '{
    "model": "claude-3-opus-20240229",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, Claude!"}
    ]
  }'`,
  },
  realtime: {
    description:
      "The Realtime API enables low-latency, bidirectional communication via WebSocket. Ideal for voice assistants and real-time interactive applications.",
    curlExample: `# WebSocket connection (use wscat or similar tool)
wscat -c "wss://api.example.com/v1/realtime?model=gpt-4o-realtime-preview" \\
  -H "Authorization: Bearer YOUR_API_KEY"

# Send message after connecting:
{"type": "response.create", "response": {"modalities": ["text"]}}`,
  },
  videos: {
    description:
      "The Video Generation API creates videos from text descriptions or images. Specify prompts and parameters to generate video content.",
    curlExample: `curl https://oneapi.laisky.com/v1/videos \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "video-generation-model",
    "prompt": "A cat playing piano",
    "duration": 5
  }'`,
  },
};

export const ChannelEndpointSettings = ({
  form,
  allEndpoints,
  defaultEndpoints,
  tr,
}: ChannelEndpointSettingsProps) => {
  const [selectedEndpoint, setSelectedEndpoint] = useState<EndpointInfo | null>(
    null
  );
  const currentEndpoints = form.watch("config.supported_endpoints") || [];

  // Determine if we're using custom endpoints or defaults
  const isUsingDefaults = currentEndpoints.length === 0;

  // Get the effective endpoints (either custom or defaults)
  const effectiveEndpoints = isUsingDefaults
    ? defaultEndpoints
    : currentEndpoints;

  const handleEndpointToggle = (endpointName: string, checked: boolean) => {
    // If currently using defaults and user makes a change, switch to custom mode
    let newEndpoints: string[];
    if (isUsingDefaults) {
      // Initialize with current defaults, then apply the change
      newEndpoints = checked
        ? [...defaultEndpoints, endpointName]
        : defaultEndpoints.filter((e) => e !== endpointName);
    } else {
      // Already in custom mode
      newEndpoints = checked
        ? [...currentEndpoints, endpointName]
        : currentEndpoints.filter((e) => e !== endpointName);
    }
    form.setValue("config.supported_endpoints", newEndpoints, {
      shouldDirty: true,
    });
  };

  const resetToDefaults = () => {
    form.setValue("config.supported_endpoints", [], {
      shouldDirty: true,
    });
  };

  const selectAll = () => {
    form.setValue(
      "config.supported_endpoints",
      allEndpoints.map((e) => e.name),
      { shouldDirty: true }
    );
  };

  const selectNone = () => {
    form.setValue("config.supported_endpoints", ["chat_completions"], {
      shouldDirty: true,
    });
  };

  if (allEndpoints.length === 0) {
    return null;
  }

  const getEndpointDoc = (name: string) => {
    return (
      ENDPOINT_DOCS[name] || {
        description: "No detailed documentation available for this endpoint.",
        curlExample: "# No example available",
      }
    );
  };

  return (
    <div className="space-y-4">
      {/* Documentation Modal */}
      <Dialog
        open={!!selectedEndpoint}
        onOpenChange={(open) => !open && setSelectedEndpoint(null)}
      >
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
          {selectedEndpoint && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  {tr(
                    `endpoints.${selectedEndpoint.name}.label`,
                    selectedEndpoint.description
                  )}
                </DialogTitle>
                <DialogDescription className="font-mono text-xs">
                  {selectedEndpoint.path}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 mt-4">
                <div>
                  <h4 className="font-medium mb-2">
                    {tr("endpoints.modal.description", "Description")}
                  </h4>
                  <p className="text-sm text-muted-foreground">
                    {tr(
                      `endpoints.${selectedEndpoint.name}.description`,
                      getEndpointDoc(selectedEndpoint.name).description
                    )}
                  </p>
                </div>
                <div>
                  <h4 className="font-medium mb-2">
                    {tr("endpoints.modal.example", "cURL Example")}
                  </h4>
                  <pre className="bg-muted p-4 rounded-md overflow-x-auto text-xs">
                    <code>
                      {getEndpointDoc(selectedEndpoint.name).curlExample}
                    </code>
                  </pre>
                </div>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>

      <div className="flex items-center justify-between">
        <LabelWithHelp
          label={tr("endpoints.label", "Supported Endpoints")}
          help={tr(
            "endpoints.help",
            "Select which API endpoints this channel supports. Click on an endpoint's info icon for details and usage examples."
          )}
        />
        <div className="flex items-center gap-2">
          {!isUsingDefaults && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={resetToDefaults}
            >
              {tr("endpoints.reset_defaults", "Reset to Defaults")}
            </Button>
          )}
          <Button type="button" variant="outline" size="sm" onClick={selectAll}>
            {tr("endpoints.select_all", "Select All")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={selectNone}
          >
            {tr("endpoints.select_none", "Minimal")}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {allEndpoints.map((endpoint) => {
          const isChecked = effectiveEndpoints.includes(endpoint.name);
          const isDefault = defaultEndpoints.includes(endpoint.name);

          return (
            <FormField
              key={endpoint.name}
              control={form.control}
              name="config.supported_endpoints"
              render={() => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0 rounded-md border p-4">
                  <FormControl>
                    <Checkbox
                      checked={isChecked}
                      onCheckedChange={(checked) =>
                        handleEndpointToggle(endpoint.name, checked === true)
                      }
                    />
                  </FormControl>
                  <div className="space-y-1 leading-none flex-1">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium text-sm">
                        {tr(
                          `endpoints.${endpoint.name}.label`,
                          endpoint.description
                        )}
                      </span>
                      <div className="flex items-center gap-1">
                        {isDefault && (
                          <span className="text-xs text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
                            {tr("endpoints.default_badge", "default")}
                          </span>
                        )}
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-6 w-6 p-0"
                          onClick={() => setSelectedEndpoint(endpoint)}
                        >
                          <Info className="h-4 w-4 text-muted-foreground hover:text-foreground" />
                        </Button>
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground font-mono">
                      {endpoint.path}
                    </p>
                  </div>
                </FormItem>
              )}
            />
          );
        })}
      </div>
    </div>
  );
};
