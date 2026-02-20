package openai

import (
	"fmt"

	"github.com/Laisky/errors/v2"

	imgutil "github.com/songquanpeng/one-api/common/image"
)

// getImageFromURLFn is injectable for tests.
var getImageFromURLFn = imgutil.GetImageFromUrl

// toDataURL downloads an image and returns a data URL string.
func toDataURL(url string) (string, error) {
	mime, data, err := getImageFromURLFn(url)
	if err != nil {
		return "", errors.Wrap(err, "get image from url")
	}
	if mime == "" {
		mime = "image/jpeg"
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, data), nil
}
