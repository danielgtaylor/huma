package main

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Item tracks the price of a good.
type Item struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   float32 `json:"price"`
	IsOffer bool    `json:"is_offer,omitempty"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.Use(gin.Recovery())

	d := func(c *gin.Context) string {
		return strings.Split(c.GetHeader("authorization"), " ")[0]
	}

	g.GET("/items/:id", func(c *gin.Context) {
		tmp := c.Param("id")
		id, err := strconv.Atoi(tmp)
		if err != nil {
			c.AbortWithError(400, err)
		}

		authInfo := d(c)

		c.Header("x-authinfo", authInfo)
		c.JSON(200, &Item{
			ID:      id,
			Name:    "Hello",
			Price:   1.25,
			IsOffer: false,
		})
	})

	g.Run("0.0.0.0:8888")
}
