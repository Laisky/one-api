package openai

// chatStreamIdentity remembers the Responses response identity so every
// converted Chat Completions chunk carries the same id, created and model.
// Delta events carry no response object; without this the first content
// chunks reached clients as "id":"","created":0,"model":"".
type chatStreamIdentity struct {
	id      string
	created int64
	model   string
}

// observe records the first non-empty identity fields of a converted
// Responses event. Parameter chunk is the event's response view and may be
// nil. It returns nothing.
func (i *chatStreamIdentity) observe(chunk *ResponseAPIResponse) {
	if chunk == nil {
		return
	}
	if i.id == "" && chunk.Id != "" {
		i.id = chunk.Id
	}
	if i.created == 0 && chunk.CreatedAt != 0 {
		i.created = chunk.CreatedAt
	}
	if i.model == "" && chunk.Model != "" {
		i.model = chunk.Model
	}
}

// stamp fills a converted chunk's missing identity fields from the observed
// identity, leaving fields the chunk already carries unchanged. Parameter
// chunk may be nil. It returns nothing.
func (i *chatStreamIdentity) stamp(chunk *ChatCompletionsStreamResponse) {
	if chunk == nil {
		return
	}
	if chunk.Id == "" {
		chunk.Id = i.id
	}
	if chunk.Created == 0 {
		chunk.Created = i.created
	}
	if chunk.Model == "" {
		chunk.Model = i.model
	}
}
