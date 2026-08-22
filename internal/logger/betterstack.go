package logger

import (
	"log/slog"

	slogbetterstack "github.com/samber/slog-betterstack"
)

// Fields Better Stack should show as top-level columns. The default
// converter buries every attr under extra, which is why Live Tail
// only showed the message text.
var betterStackRootKeys = []string{
	"email",
	"userEmail",
	"tag",
	"method",
	"path",
	"status",
	"org_id",
	"user_id",
	"request_id",
	"hold_id",
	"hold_micros",
	"amount_micros",
	"actual_micros",
	"refund_micros",
	"remaining_before_micros",
	"remaining_after_micros",
	"input_tokens",
	"output_tokens",
}

func betterStackConverter(
	addSource bool,
	replaceAttr func(groups []string, a slog.Attr) slog.Attr,
	loggerAttr []slog.Attr,
	groups []string,
	record *slog.Record,
) map[string]any {
	payload := slogbetterstack.DefaultConverter(addSource, replaceAttr, loggerAttr, groups, record)
	extra, _ := payload[slogbetterstack.ContextKey].(map[string]any)
	if extra == nil {
		return payload
	}
	for _, key := range betterStackRootKeys {
		if value, ok := extra[key]; ok {
			payload[key] = value
		}
	}
	return payload
}
