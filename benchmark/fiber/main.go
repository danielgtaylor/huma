package main

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber"
	"github.com/gofiber/fiber/middleware"
)

// Item tracks the price of a good.
type Item struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   float32 `json:"price"`
	IsOffer bool    `json:"is_offer,omitempty"`
}

func main() {
	app := fiber.New()
	app.Use(middleware.Recover())

	d := func(c *fiber.Ctx) string {
		return strings.Split(c.Get("authorization"), " ")[0]
	}

	app.Get("/items/:id", func(c *fiber.Ctx) {
		tmp := c.Params("id")
		id, err := strconv.Atoi(tmp)
		if err != nil {
			c.Status(500)
			return
		}

		authInfo := d(c)

		c.Set("x-authinfo", authInfo)
		c.Status(200)
		c.JSON(&Item{
			ID:      id,
			Name:    "Hello",
			Price:   1.25,
			IsOffer: false,
		})
	})

	app.Listen("127.0.0.1:8000")
}
