package pullrequest

import "net/http"

var requestChangesSpec = actionSpec{
	name:     "request-changes",
	aliases:  []string{"requestChanges", "requestchanges"},
	short:    "Request changes on a pullrequest by its <pullrequest-id>. If not provided, it will try to request changes on the only open pullrequest.",
	whatIf:   "Requesting changes on",
	errVerb:  "request changes on",
	endpoint: "request-changes",
	method:   http.MethodPost,
	logFetch: true,
}

func init() {
	Command.AddCommand(newActionCommand(requestChangesSpec))
}
