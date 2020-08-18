package huma

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// splitDocs will split a single string out into a title/description combo.
func splitDocs(docs string) (title, desc string) {
	title = docs
	desc = ""
	if strings.Contains(docs, "\n") {
		parts := strings.SplitN(docs, "\n", 2)
		title = parts[0]
		desc = parts[1]
	}
	return
}

// RapiDocTemplate is the template used to generate the RapiDoc.  It needs two args to render:
// 1. the title
// 2. the path to the openapi.yaml file
var RapiDocTemplate = `<!doctype html>
<html>
<head>
	<title>%s</title>
  <meta charset="utf-8">
  <script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
</head>
<body>
  <rapi-doc
		spec-url="%s"
		render-style="read"
    show-header="false"
    primary-color="#f74799"
    nav-accent-color="#47afe8"
  > </rapi-doc>
</body>
</html>`

// ReDocTemplate is the template used to generate the RapiDoc.  It needs two args to render:
// 1. the title
// 2. the path to the openapi.yaml file
var ReDocTemplate = `<!DOCTYPE html>
<html>
  <head>
    <title>%s</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
		<style>body { margin: 0; padding: 0; }</style>
  </head>
  <body>
    <redoc spec-url='%s'></redoc>
    <script src="https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"> </script>
  </body>
</html>`

var SwaggerUITemplate = `<!-- HTML for static distribution bundle build -->
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <title>%s</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/3.24.2/swagger-ui.css" >
    <style>
      html
      {
        box-sizing: border-box;
        overflow: -moz-scrollbars-vertical;
        overflow-y: scroll;
      }

      *,
      *:before,
      *:after
      {
        box-sizing: inherit;
      }

      body
      {
        margin:0;
        background: #fafafa;
      }
    </style>
  </head>

  <body>
    <div id="swagger-ui"></div>

    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/3.24.2/swagger-ui-bundle.js"> </script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/3.24.2/swagger-ui-standalone-preset.js"> </script>
    <script>
    window.onload = function() {
      // Begin Swagger UI call region
      const ui = SwaggerUIBundle({
        url: "%s",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      })
      // End Swagger UI call region

      window.ui = ui
    }
  </script>
  </body>
</html>`

// RapiDocHandler renders documentation using RapiDoc.
func RapiDocHandler(pageTitle string) Handler {
	return func(c *gin.Context) {
		c.Data(200, "text/html", []byte(fmt.Sprintf(RapiDocTemplate, pageTitle, "/openapi.json")))
	}
}

// ReDocHandler renders documentation using ReDoc.
func ReDocHandler(pageTitle string) Handler {
	return func(c *gin.Context) {
		c.Data(200, "text/html", []byte(fmt.Sprintf(ReDocTemplate, pageTitle, "/openapi.json")))
	}
}

// SwaggerUIHandler renders documentation using Swagger UI.
func SwaggerUIHandler(pageTitle string) Handler {
	return func(c *gin.Context) {
		c.Data(200, "text/html", []byte(fmt.Sprintf(SwaggerUITemplate, pageTitle, "/openapi.json")))
	}
}
