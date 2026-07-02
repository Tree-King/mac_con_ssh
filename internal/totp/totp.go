package totp

import (
	"time"

	"github.com/pquerna/otp/totp"
)

func Generate(seed string) (string, error) { return totp.GenerateCode(seed, now()) }

var now = func() time.Time { return time.Now() }
