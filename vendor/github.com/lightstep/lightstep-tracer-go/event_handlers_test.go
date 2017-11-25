package lightstep

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type handler1 struct {
	foo int
}

func (h handler1) OnEvent(event Event) {
}

type handler2 struct {
	bar string
}

func (h handler2) OnEvent(event Event) {}

var _ = Describe("SetGlobalEventHandler", func() {
	var h1 handler1
	var h2 handler2
	BeforeEach(func() {
		h1 = handler1{foo: 1}
		h2 = handler2{bar: "bar"}
	})

	It("can be updated with different types of event handlers", func() {
		Expect(func() { SetGlobalEventHandler(h1.OnEvent) }).NotTo(Panic())
		Expect(func() { SetGlobalEventHandler(h2.OnEvent) }).NotTo(Panic())
	})

	It("does not race when setting and calling the handler", func() {
		for i := 0; i < 100; i++ {
			go func() {
				SetGlobalEventHandler(h1.OnEvent)
			}()
			go func() {
				emitEvent(newEventStartError(nil))
			}()
			go func() {
				SetGlobalEventHandler(h2.OnEvent)
			}()
			go func() {
				emitEvent(newEventStartError(nil))
			}()
		}
		time.Sleep(time.Millisecond)
	})
})
