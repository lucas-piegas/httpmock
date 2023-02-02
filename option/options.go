package option

import (
	"time"

	"go.uber.org/zap"
)

type HttpMockOptionFunc func(*HttpMockOptions) error

type HttpMockOptions struct {
	Delay time.Duration
}

func WithResponseDelay(delay time.Duration) HttpMockOptionFunc {
	return func(o *HttpMockOptions) error {
		o.Delay = delay
		return nil
	}
}

func ProcessOptions(logger *zap.Logger, optionFunc []HttpMockOptionFunc) HttpMockOptions {

	var op HttpMockOptions

	for _, fn := range optionFunc {
		if err := fn(&op); err != nil {
			logger.Panic("load option failed", zap.Error(err))
		}
	}

	return op
}
