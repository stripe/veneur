package pointer

import (
	"github.com/signalfx/golib/timekeeper"
	"github.com/signalfx/golib/timekeeper/timekeepertest"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

type Job interface {
	Pay() int
}

type intJob int

func (j intJob) Pay() int {
	return int(j)
}

type Person struct {
	Name        *string
	Age         *int32
	Job         Job
	Coworkers   []Person
	Tk          timekeeper.TimeKeeper
	SinceWakeup *time.Duration

	Uint32 *uint32
	Uint   *uint
	Uint16 *uint16

	Int     *int
	Int64   *int64
	Bool    *bool
	Float64 *float64
}

type NotAPerson struct {
	Name        *string
	Age         *int32
	Job         Job
	Coworkers   []Person
	Tk          timekeeper.TimeKeeper
	SinceWakeup *time.Duration

	Uint32 *uint32
	Uint   *uint
	Uint16 *uint16
	Int    *int
	Int64  *int64
}

func TestFillDefaultFrom(t *testing.T) {
	Convey("An fully default person", t, func() {
		defaultPerson := Person{
			Name:        String("john"),
			Age:         Int32(21),
			Job:         intJob(100),
			Tk:          timekeeper.RealTime{},
			SinceWakeup: Duration(time.Second),
			Uint32:      Uint32(1),
			Uint:        Uint(2),
			Uint16:      Uint16(3),
			Int:         Int(5),
			Int64:       Int64(6),
			Bool:        Bool(true),
			Float64:     Float64(4.0),
			Coworkers: []Person{
				{
					Name: String("jack"),
					Age:  Int32(33),
					Job:  intJob(101),
				},
			},
		}
		Convey("should fill", func() {
			p := FillDefaultFrom(&defaultPerson).(*Person)
			So(*p.Age, ShouldEqual, 21)
			So(*p.Name, ShouldEqual, "john")
			So(*p.SinceWakeup, ShouldEqual, time.Second)
			So(*p.Uint32, ShouldEqual, 1)
			So(*p.Uint, ShouldEqual, 2)
			So(*p.Uint16, ShouldEqual, 3)
			So(*p.Int, ShouldEqual, 5)
			So(*p.Int64, ShouldEqual, 6)
			So(p.Job.Pay(), ShouldEqual, 100)
			So(len(p.Coworkers), ShouldEqual, 1)
			So(*p.Bool, ShouldBeTrue)
			So(*p.Float64, ShouldEqual, 4.0)
			So(*p.Coworkers[0].Name, ShouldEqual, "jack")
			So(p.Tk, ShouldHaveSameTypeAs, timekeeper.RealTime{})
		})
		Convey("should be able to override defaults", func() {
			tkStub := timekeepertest.NewStubClock(time.Now())
			p := FillDefaultFrom(&Person{Age: Int32(22), Tk: tkStub}, &defaultPerson).(*Person)
			So(*p.Age, ShouldEqual, 22)
			So(*p.Name, ShouldEqual, "john")
			So(p.Job.Pay(), ShouldEqual, 100)
			So(p.Tk, ShouldEqual, tkStub)
		})
		Convey("should allow nil defaults", func() {
			defaultPerson.Age = nil
			p := FillDefaultFrom(&defaultPerson).(*Person)
			So(p.Age, ShouldBeNil)
			So(*p.Name, ShouldEqual, "john")
		})
		Convey("should allow nil elements", func() {
			var nilPerson *Person
			p := FillDefaultFrom(nilPerson, &defaultPerson).(*Person)
			So(*p.Age, ShouldEqual, 21)
		})
		Convey("should allow nil middle elements", func() {
			var nilPerson *Person
			p := FillDefaultFrom(nilPerson, nil, &defaultPerson).(*Person)
			So(*p.Age, ShouldEqual, 21)
		})
		Convey("should allow chaining", func() {
			firstDefault := Person{
				Name: String("jackie"),
			}
			p := FillDefaultFrom(&firstDefault, &defaultPerson).(*Person)
			So(*p.Age, ShouldEqual, 21)
			So(*p.Name, ShouldEqual, "jackie")
			Convey("and not modify structs", func() {
				So(*firstDefault.Name, ShouldEqual, "jackie")
				So(*defaultPerson.Name, ShouldEqual, "john")
				So(firstDefault.Age, ShouldBeNil)
				So(*defaultPerson.Age, ShouldEqual, 21)
			})
		})
		Convey("should work with empty lists", func() {
			So(FillDefaultFrom(), ShouldBeNil)
		})
		Convey("should catch panics", func() {
			So(func() {
				FillDefaultFrom(nil, &Person{})
			}, ShouldPanic)
			So(func() {
				FillDefaultFrom(&Person{}, &NotAPerson{})
			}, ShouldPanic)
			So(func() {
				FillDefaultFrom(Person{})
			}, ShouldPanic)
			So(func() {
				FillDefaultFrom(&Person{}, Person{})
			}, ShouldPanic)
			So(func() {
				FillDefaultFrom(&Person{}, &Person{}, &NotAPerson{})
			}, ShouldPanic)
		})
	})
}
