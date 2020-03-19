package main

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Item tracks the price of a good.
type Item struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   float32 `json:"price"`
	IsOffer bool    `json:"is_offer,omitempty"`
}

func main() {
	e := echo.New()

	d := func(c echo.Context) string {
		return strings.Split(c.Request().Header.Get("authorization"), " ")[0]
	}

	e.GET("/items/:id", func(c echo.Context) error {
		tmp := c.Param("id")
		id, err := strconv.Atoi(tmp)
		if err != nil {
			return err
		}

		authInfo := d(c)

		c.Response().Header().Add("x-authinfo", authInfo)
		c.JSON(200, &Item{
			ID:      id,
			Name:    "Hello",
			Price:   1.25,
			IsOffer: false,
		})
		return nil
	})

	e.Logger.Fatal(e.Start(":1323"))
}
