package huma

import "testing"

func TestDowngradeSchemaRefHelpersIgnoreUnsupportedShapes(t *testing.T) {
	downgradeSchemaRefs([]any{})
	downgradeSchemaRefs(map[string]any{})
	downgradePathItemSchemas("path item")
	downgradeOperationSchemas("operation")
	downgradeOperationSchemas(map[string]any{})
	downgradeCallbackSchemas("callback")
	downgradeParamSchemas("parameter")
	downgradeRequestBodySchemas("request body")
	downgradeResponseSchemas("response")
	downgradeContentSchemas("content")
	downgradeMediaTypeSchemas("media type")
	downgradeSchema("schema")
}
