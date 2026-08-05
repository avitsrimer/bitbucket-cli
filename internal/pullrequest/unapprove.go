package pullrequest

import "net/http"

var unapproveSpec = actionSpec{
	name:     "unapprove",
	short:    "unapprove a pullrequest by its <pullrequest-id>. If not provided, it will try to unapprove the only open pullrequest.",
	whatIf:   "Unapproving",
	errVerb:  "unapprove",
	endpoint: "approve",
	method:   http.MethodDelete,
}

func init() {
	Command.AddCommand(newActionCommand(unapproveSpec))
}
