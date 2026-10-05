package asyncvideo

import (
	"net/url"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
)

// ValidateObservation enforces the gateway result contract for EVERY provider.
// Unknown or malformed observations cannot close a financial hold or change an
// established failure into success. Such errors are retried as safe GETs only.
func ValidateObservation(previous string, observation Observation) error {
	switch observation.State {
	case model.AsyncTaskQueued, model.AsyncTaskRunning, model.AsyncTaskCompleted, model.AsyncTaskFailed, model.AsyncTaskCancelled:
	default:
		return errors.New("invalid async video observation state")
	}
	if (previous == model.AsyncTaskFailed || previous == model.AsyncTaskCancelled) && observation.State != previous {
		return errors.New("provider changed a terminal task outcome")
	}
	if observation.Refunded && observation.State != model.AsyncTaskFailed && observation.State != model.AsyncTaskCancelled {
		return errors.New("refund conflicts with task outcome")
	}
	if observation.State != model.AsyncTaskCompleted {
		return nil
	}
	if observation.Result == nil || len(observation.Result.Videos) == 0 || len(observation.Result.Videos) > 32 {
		return errors.New("completed video task needs a bounded result")
	}
	for _, video := range observation.Result.Videos {
		parsed, err := url.Parse(video.URL)
		if err != nil || len(video.URL) > 8192 || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return errors.New("invalid completed video URL")
		}
	}
	return nil
}
