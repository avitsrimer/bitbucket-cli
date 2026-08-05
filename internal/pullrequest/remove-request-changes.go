package pullrequest

import "net/http"

var removeRequestChangesSpec = actionSpec{
	name:     "remove-request-changes",
	aliases:  []string{"removeRequestChanges", "remove-requestChanges", "removerequestchanges", "cancel-request-changes"},
	short:    "Remove request changes on a pullrequest by its <pullrequest-id>. If not provided, it will try to remove request changes on the only open pullrequest.",
	whatIf:   "Removing request changes on",
	errVerb:  "remove request changes on",
	endpoint: "request-changes",
	method:   http.MethodDelete,
	logFetch: true,
}

func init() {
	Command.AddCommand(newActionCommand(removeRequestChangesSpec))
}
