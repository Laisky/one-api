package relay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/Laisky/one-api/relay/apitype"
)

func TestGetAdaptor(t *testing.T) {
	Convey("get adaptor", t, func() {
		for i := range apitype.Dummy {
			a := GetAdaptor(i)
			So(a, ShouldNotBeNil)
		}
	})
}
