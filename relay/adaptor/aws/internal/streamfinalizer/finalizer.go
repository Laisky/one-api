package streamfinalizer

import (
	"encoding/json"

	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

type Renderer func([]byte) bool

type Logger interface {
	Error(msg string, fields ...zap.Field)
}

type Finalizer struct {
	model       string
	createdTime int64
	usage       *relaymodel.Usage
	render      Renderer
	logger      Logger

	id               string
	stopReason       *string
	stopReceived     bool
	metadataReceived bool
	finalSent        bool
}

func NewFinalizer(model string, createdTime int64, usage *relaymodel.Usage, logger Logger, render Renderer) *Finalizer {
	if usage == nil {
		usage = &relaymodel.Usage{}
	}

	return &Finalizer{
		model:       model,
		createdTime: createdTime,
		usage:       usage,
		render:      render,
		logger:      logger,
	}
}

func (f *Finalizer) SetID(id string) {
	f.id = id
}

func (f *Finalizer) RecordStop(stopReason *string) bool {
	f.stopReason = stopReason
	f.stopReceived = true
	return f.emitFinal(false)
}

func (f *Finalizer) RecordMetadata(streamUsage *types.TokenUsage) bool {
	if streamUsage != nil {
		if streamUsage.InputTokens != nil {
			f.usage.PromptTokens = int(*streamUsage.InputTokens)
		}
		if streamUsage.OutputTokens != nil {
			f.usage.CompletionTokens = int(*streamUsage.OutputTokens)
		}
		if streamUsage.TotalTokens != nil {
			f.usage.TotalTokens = int(*streamUsage.TotalTokens)
		}
	}
	f.metadataReceived = true
	return f.emitFinal(false)
}

func (f *Finalizer) FinalizeOnClose() bool {
	return f.emitFinal(true)
}

func (f *Finalizer) HasEmittedFinalChunk() bool {
	return f.finalSent
}

func (f *Finalizer) emitFinal(force bool) bool {
	if f.finalSent {
		return true
	}
	if f.id == "" {
		return true
	}
	if !force {
		if !f.stopReceived {
			return true
		}
		if !f.metadataReceived {
			return true
		}
	}

	response := &openai.ChatCompletionsStreamResponse{
		Id:      f.id,
		Object:  "chat.completion.chunk",
		Created: f.createdTime,
		Model:   f.model,
		Choices: []openai.ChatCompletionsStreamResponseChoice{
			{
				Index:        0,
				Delta:        relaymodel.Message{},
				FinishReason: f.stopReason,
			},
		},
	}
	if f.metadataReceived && f.usage != nil {
		response.Usage = f.usage
	}

	payload, err := json.Marshal(response)
	if err != nil {
		if f.logger != nil {
			f.logger.Error("error marshalling final stream response", zap.Error(err))
		}
		return false
	}

	if f.render != nil && !f.render(payload) {
		return false
	}

	f.finalSent = true
	return true
}
