package sfxtest

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestForcedErr(t *testing.T) {
	Convey("Given an error and names", t, func() {
		error := errors.New("my error")
		names := []string{"foo", "bar"}
		Convey("ForcedError returns a ErrCheck", func() {
			errCheck := ForcedError(error, names...)
			Convey("That returns error when one of names is specified", func() {
				So(errCheck(names[0]), ShouldEqual, error)
			})
			Convey("That does not return error when a string not in names is specified", func() {
				So(errCheck(names[0]+"moo"), ShouldBeNil)
			})
		})
	})
}

type errCheckerTest struct {
	ErrChecker
	name string
}

func (ect *errCheckerTest) GetName() (string, error) {
	if err := ect.CheckForError("GetName"); err != nil {
		return "", err
	}
	return ect.name, nil
}

func (ect *errCheckerTest) SetName(name string) error {
	if err := ect.CheckForError("SetName"); err != nil {
		return err
	}
	ect.name = name
	return nil
}

func TestSetErrorCheck(t *testing.T) {
	Convey("Given a struct using ErrChecker and an ErrCheck", t, func() {
		ect := &errCheckerTest{name: "hi"}
		error := errors.New("my error")
		errCheck := ForcedError(error, "GetName")
		Convey("SetErrorCheck(ErrCheck) will make struct use ErrCheck", func() {
			ect.SetErrorCheck(errCheck)
			_, err := ect.GetName()
			So(err, ShouldEqual, err)
		})
	})
}

func TestCheckForError(t *testing.T) {
	Convey("Given a struct using ErrChecker and an ErrCheck", t, func() {
		ect := &errCheckerTest{name: "hi"}
		error := errors.New("my error")
		Convey("With ErrCheck set CheckForErrors uses the ErrCheck", func() {
			ect.SetErrorCheck(ForcedError(error, "GetName"))
			_, err := ect.GetName()
			So(err, ShouldEqual, err)
			So(ect.SetName("new name"), ShouldBeNil)
		})
		Convey("Without ErrCheck set CheckForErrors returns nil", func() {
			_, err := ect.GetName()
			So(err, ShouldBeNil)
			So(ect.SetName("new name"), ShouldBeNil)
		})
	})
}
