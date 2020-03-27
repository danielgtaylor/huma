package huma

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// RapiDocHandler renders documentation using RapiDoc.
func RapiDocHandler(c *gin.Context, api *OpenAPI) {
	c.Data(200, "text/html", []byte(fmt.Sprintf(`<!doctype html>
<html>
<head>
	<title>%s</title>
  <meta charset="utf-8">
  <script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
</head>
<body>
  <rapi-doc
		spec-url="/openapi.json"
		render-style="read"
		show-header="false"
  > </rapi-doc>
</body>
</html>`, api.Title)))
}

// ReDocHandler renders documentation using ReDoc.
func ReDocHandler(c *gin.Context, api *OpenAPI) {
	c.Data(200, "text/html", []byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <title>%s</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
		<style>body { margin: 0; padding: 0; }</style>
  </head>
  <body>
    <redoc spec-url='/openapi.json'></redoc>
    <script src="https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"> </script>
  </body>
</html>`, api.Title)))
}

// SwaggerUIHandler renders documentation using Swagger UI.
func SwaggerUIHandler(c *gin.Context, api *OpenAPI) {
	c.Data(200, "text/html", []byte(fmt.Sprintf(`<!-- HTML for static distribution bundle build -->
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
        url: "/openapi.json",
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
</html>`, api.Title)))
}
