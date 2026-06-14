package channeltype

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChannelBaseURLs(t *testing.T) {
	Convey("channel base urls", t, func() {
		So(len(ChannelBaseURLs), ShouldEqual, Dummy)
	})
}

func TestNVIDIAChannelBaseURLIsEditable(t *testing.T) {
	Convey("nvidia base url config", t, func() {
		cfg := GetChannelBaseURLConfig(NVIDIA)
		So(cfg.URL, ShouldEqual, "https://integrate.api.nvidia.com/v1")
		So(cfg.Editable, ShouldBeTrue)
	})
}
