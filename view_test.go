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
			v, err := Render(u, "admin")
			So(err, ShouldBeNil)
			So(v, ShouldResemble, map[string]interface{}{
				"Id": uint(7),
			})
		})
		Convey("Field can included in many views", func() {
			v, err := Render(u, "support")
			So(err, ShouldBeNil)
			So(v, ShouldResemble, map[string]interface{}{
				"Id":   uint(7),
				"Name": "Jon Doe",
			})
		})
	})

	Convey("It does not omit fields if no one field matched", test, func() {
		v, err := Render(u, "system")
		So(err, ShouldBeNil)
		So(v, ShouldResemble, u)
	})

	Convey("It does not convert types and does not map data if it is possible", test, func() {
		v, err := Render(u, "system")
		So(err, ShouldBeNil)
		So(v, ShouldEqual, u)
	})

	Convey("It process the complex structures of data", test, func() {
		a := &Activity{
			u, []Product{
				{3, "T-shirt", "123-456-7890"},
				{5, "Shoes", "789-000-1111"},
			},
		}
		v, err := Render(a, "admin")
		So(err, ShouldBeNil)
		So(v, ShouldResemble, map[string]interface{}{
			"Products": []Product{
				{Id: uint(3), Name: "T-shirt", Code: "123-456-7890"},
				{Id: uint(5), Name: "Shoes", Code: "789-000-1111"},
			},
			"User": map[string]interface{}{"Id": uint(7)},
		})
	})

	Convey("It returns UnsupportedTypeError when attempting to process an unsupported value type", test, func() {
		var src struct {
			Field struct {
				Channel chan interface{}
			}
		}
		v, err := Render(src, "admin")
		So(err, ShouldNotBeNil)
		So(err, ShouldHaveSameTypeAs, &UnsupportedTypeError{})
		So(v, ShouldBeNil)
	})

	Convey("It converts maps correctly", test, func() {
		var src = map[int]*Product{
			1: {3, "T-shirt", "123-456-7890"},
		}
		v, err := Render(src, "support")
		So(err, ShouldBeNil)
		So(v, ShouldResemble, map[int]interface{}{
			1: map[string]interface{}{"Name": "T-shirt", "Code": "123-456-7890"},
		})
	})

	Convey("It converts arrays correctly", test, func() {
		var src = [2]*Product{
			{3, "T-shirt", "123-456-7890"},
			{5, "Shoes", "789-000-1111"},
		}
		v, err := Render(src, "support")
		So(err, ShouldBeNil)
		So(v, ShouldResemble, []interface{}{
			map[string]interface{}{"Name": "T-shirt", "Code": "123-456-7890"},
			map[string]interface{}{"Name": "Shoes", "Code": "789-000-1111"},
		})
	})

	Convey("It converts interfaces correctly", test, func() {
		var src = struct {
			I interface{}
		}{u}
		v, err := Render(src, "admin")
		So(err, ShouldBeNil)
		So(v, ShouldResemble, map[string]interface{}{"I": map[string]interface{}{"Id": uint(7)}})

		src.I = struct {
			S string `view:"admin"`
			K string
		}{"test", "key"}
		v, err = Render(src, "admin")
		So(err, ShouldBeNil)
		So(v, ShouldResemble, map[string]interface{}{"I": map[string]interface{}{"S": "test"}})
	})

	Convey("Name conversion", test, func() {

	})
}

func BenchmarkRender(bench *testing.B) {
	a := &Activity{
		&User{7, "Jon Doe", "secret", "12345"},
		[]Product{
			{3, "T-shirt", "123-456-7890"},
			{5, "Shoes", "789-000-1111"},
		},
	}
	for i := 0; i < bench.N; i++ {
		Render(a, "admin")
	}
}
