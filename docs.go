package huma

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// ReDocHandler renders documentation using ReDoc.
func (r *Router) ReDocHandler(c *gin.Context) {
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
</html>`, r.api.Title)))
}

// RapiDocHandler renders documentation using RapiDoc.
func (r *Router) RapiDocHandler(c *gin.Context) {
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
		schema-style="table"
  > </rapi-doc>
</body>
</html>`, r.api.Title)))
}
