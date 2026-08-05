package pullrequest

import "net/http"

var approveSpec = actionSpec{
	name:     "approve",
	short:    "approve a pullrequest by its <pullrequest-id>. If not provided, it will try to approve the only open pullrequest.",
	whatIf:   "Approving",
	errVerb:  "approve",
	endpoint: "approve",
	method:   http.MethodPost,
	logFetch: true,
}

func init() {
	Command.AddCommand(newActionCommand(approveSpec))
}
