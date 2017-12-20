package log

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func BenchmarkNewHierarchy(b *testing.B) {
	counter := &Counter{}
	p1 := NewHierarchy(counter)
	p2 := p1.CreateChild()
	for i := 0; i < b.N; i++ {
		p2.Log("hello", "world")
	}
}

func TestHierarchy(t *testing.T) {
	Convey("hierarchy logger", t, func() {
		counter := &Counter{}
		p1 := NewHierarchy(counter)
		Convey("should be able to setup", func() {
			p1.setupFromEnv(func(string) string { return "" })
			p1.Log("hello", "world")
			So(counter.Count, ShouldEqual, 1)
			p1.setupFromEnv(func(string) string { return "/dev/null" })

			p1.Log("hello", "world")
			So(counter.Count, ShouldEqual, 1)
		})
		Convey("Should log", func() {
			p1.Log("hello")
			So(counter.Count, ShouldEqual, 1)
		})
		Convey("Should use children", func() {
			p2 := p1.CreateChild()
			p2.Log("hello")
			So(counter.Count, ShouldEqual, 1)

			p1.Log("hello")
			So(counter.Count, ShouldEqual, 2)
		})
		Convey("Should allow changing", func() {
			p2 := p1.CreateChild()
			counter2 := &Counter{}
			p1.Set(counter2)
			p2.Log()
			So(counter.Count, ShouldEqual, 0)
			So(counter2.Count, ShouldEqual, 1)
		})
		Convey("should support empty struct", func() {
			h1 := Hierarchy{}
			h1.Log("hello", "world")
		})
		Convey("Should allow disabling", func() {
			g1 := &Gate{Logger: counter}
			p1.Set(g1)
			g1.Disable()
			p1.Log()

			So(counter.Count, ShouldEqual, 0)
			So(IsDisabled(p1), ShouldBeTrue)
		})
	})
}

func BenchmarkNewHierarchyContext(b *testing.B) {
	counter := &Counter{}
	p1 := NewHierarchy(counter)
	p2 := p1.CreateChild()
	ctx := NewContext(p2)
	for i := 0; i < b.N; i++ {
		ctx.Log("hello", "world")
	}
}

func BenchmarkNewHierarchyDisabled(b *testing.B) {
	counter := &Counter{}
	p1 := NewHierarchy(counter)
	p2 := p1.CreateChild()
	ctx := NewContext(p2)
	p1.Set(Discard)
	for i := 0; i < b.N; i++ {
		ctx.Log("hello", "world")
	}
}
