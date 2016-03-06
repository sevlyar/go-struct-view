package view

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

type User struct {
	Id       uint   `view:"support,admin"`
	Name     string `view:"support,user"`
	Password string `view:"user"`
	Key      string
}

type Product struct {
	Id   uint
	Name string `view:"support, user"`
	Code string `view:"support"`
}

type Activity struct {
	User     *User
	Products []Product
}

func TestRender(test *testing.T) {
	u := &User{
		Id:       7,
		Name:     "Jon Doe",
		Password: "secret",
		Key:      "12345",
	}

	Convey("It omits unmatched fields and converts struct to map if at least one field matched", test, func() {
		Convey("Field matches by tag", func() {
			So(Render(u, "admin"), ShouldResemble, map[string]interface{}{
				"Id": uint(7),
			})
		})
		Convey("Field can included in many views", func() {
			So(Render(u, "support"), ShouldResemble, map[string]interface{}{
				"Id":   uint(7),
				"Name": "Jon Doe",
			})
		})
	})

	Convey("It does not omit fields if no one field matched", test, func() {
		So(Render(u, "system"), ShouldResemble, u)
	})

	Convey("It does not convert types and does not map data if it is possible", test, func() {
		So(Render(u, "system"), ShouldEqual, u)
	})

	Convey("It process the complex structures of data", test, func() {
		a := &Activity{
			u, []Product{
				{3, "T-shirt", "123-456-7890"},
				{5, "Shoes", "789-000-1111"},
			},
		}
		So(Render(a, "admin"), ShouldResemble, map[string]interface{}{
			"Products": []Product{
				{Id: uint(3), Name: "T-shirt", Code: "123-456-7890"},
				{Id: uint(5), Name: "Shoes", Code: "789-000-1111"},
			},
			"User": map[string]interface{}{"Id": uint(7)},
		})
	})

	Convey("Name convertation", test, func() {

	})
}
