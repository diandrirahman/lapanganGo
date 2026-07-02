package refunds

import "time"

func SetTimeNow(f func() time.Time) {
	timeNow = f
}
