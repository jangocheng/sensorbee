package core

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/sensorbee/sensorbee.v0/data"
)

func BenchmarkPipe(b *testing.B) {
	ctx := NewContext(nil)
	r, s := newPipe("test", 1024)
	go func() {
		for _ = range r.in {
		}
	}()

	t := &Tuple{}
	t.Data = data.Map{}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Write(ctx, t)
		}
	})
}

func drainReceiver(r *pipeReceiver) {
	for _ = range r.in {
	}
}

func TestPipe(t *testing.T) {
	ctx := NewContext(nil)

	Convey("Given a pipe", t, func() {
		// Use small capacity to check the sender never blocks.
		r, s := newPipe("test", 1)
		t := &Tuple{
			InputName: "hoge",
			Data: data.Map{
				"v": data.Int(1),
			},
		}

		Convey("When sending a tuple via the sender", func() {
			So(s.Write(ctx, t), ShouldBeNil)

			Convey("Then the tuple should be received by the receiver", func() {
				rt := <-r.in

				Convey("And its value should be correct", func() {
					So(rt.Data["v"], ShouldEqual, data.Int(1))
				})

				Convey("And its input name should be overwritten", func() {
					So(rt.InputName, ShouldEqual, "test")
				})
			})
		})

		Convey("When closing the pipe via the sender", func() {
			s.Close(ctx)

			Convey("Then it cannot no longer write a tuple", func() {
				So(s.Write(ctx, t), ShouldPointTo, errPipeClosed)
			})

			Convey("Then it can be closed again although it's not a part of the specification", func() {
				// This is only for improving the coverage.
				So(func() {
					s.Close(ctx)
				}, ShouldNotPanic)
			})
		})

		Convey("When closing the pipe via the receiver", func() {
			r.close()
			go drainReceiver(r)

			Convey("Then the sender should eventually be unable to write anymore tuple", func() {
				var err error
				for {
					// It can take many times until it will stop.
					err = s.Write(ctx, t)
					if err != nil {
						break
					}
				}
				So(err, ShouldPointTo, errPipeClosed)
			})
		})

		Convey("When sending tuples with DropLatest mode", func() {
			t2 := t.Copy()
			t2.Data["v"] = data.Int(2)
			s.dropMode = DropLatest

			So(s.Write(ctx, t), ShouldBeNil)
			So(s.Write(ctx, t2), ShouldBeNil)

			Convey("Then only the first tuple should be received by the receiver", func() {
				rt := <-r.in
				So(rt.Data["v"], ShouldEqual, data.Int(1))
				So(len(r.in), ShouldEqual, 0)
			})
		})

		Convey("When sending tuples with DropOldest mode", func() {
			t2 := t.Copy()
			t2.Data["v"] = data.Int(2)
			s.dropMode = DropOldest

			So(s.Write(ctx, t), ShouldBeNil)
			So(s.Write(ctx, t2), ShouldBeNil)

			Convey("Then only the second tuple should be received by the receiver", func() {
				rt := <-r.in
				So(rt.Data["v"], ShouldEqual, data.Int(2))
				So(len(r.in), ShouldEqual, 0)
			})
		})
	})
}

func TestDataSources(t *testing.T) {
	ctx := NewContext(nil)

	Convey("Given an empty data source", t, func() {
		srcs := newDataSources(NTBox, "test_component")

		Convey("When stopping it before starting to pour tuples", func() {
			srcs.stop(ctx)

			Convey("Then it pouring should fail", func() {
				si := NewTupleCollectorSink()
				So(srcs.pour(ctx, si, 1), ShouldNotBeNil)
			})
		})
	})

	Convey("Given an empty data source", t, func() {
		srcs := newDataSources(NTBox, "test_component")
		si := NewTupleCollectorSink()

		t := &Tuple{
			InputName: "some_component",
			Data: data.Map{
				"v": data.Int(1),
			},
		}

		stopped := make(chan error, 1)
		go func() {
			stopped <- srcs.pour(ctx, si, 4)
		}()
		Reset(func() {
			srcs.stop(ctx)
		})
		srcs.state.Wait(TSRunning)

		Convey("When starting it without any input", func() {
			Convey("Then it should start pouring", func() {
				// Just check if this test isn't dead-locked.
				So(true, ShouldBeTrue)
			})

			Convey("Then the sink should receive anything", func() {
				srcs.stop(ctx)
				<-stopped
				So(si.len(), ShouldEqual, 0)
			})
		})

		Convey("When adding an input after starting pouring and write a tuple", func() {
			r, s := newPipe("test1", 1)
			So(srcs.add("test_node_1", r), ShouldBeNil)
			So(s.Write(ctx, t), ShouldBeNil)
			si.Wait(1)
			s.close()
			srcs.stop(ctx)
			<-stopped

			Convey("Then the sink receive the tuple", func() {
				So(si.len(), ShouldEqual, 1)
			})
		})
	})

	Convey("Given a data source having destinations", t, func() {
		srcs := newDataSources(NTBox, "test_component")
		dsts := make([]*pipeSender, 2)
		for i := range dsts {
			r, s := newPipe(fmt.Sprint("test", i+1), 1)
			srcs.add(fmt.Sprint("test_node_", i+1), r)
			dsts[i] = s
		}
		Reset(func() {
			for _, d := range dsts {
				d.close() // safe to call multiple times
			}
		})
		si := NewTupleCollectorSink()

		t := &Tuple{
			InputName: "some_component",
			Data: data.Map{
				"v": data.Int(1),
			},
		}

		stopped := make(chan error, 1)
		go func() {
			stopped <- srcs.pour(ctx, si, 4)
		}()
		Reset(func() {
			srcs.stop(ctx)
		})
		srcs.state.Wait(TSRunning)

		Convey("When starting it again", func() {
			err := srcs.pour(ctx, si, 4)

			Convey("Then it should fail", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When sending tuples from one source", func() {
			for i := 0; i < 5; i++ {
				So(dsts[0].Write(ctx, t), ShouldBeNil)
			}
			srcs.enableGracefulStop()
			srcs.stop(ctx)
			So(<-stopped, ShouldBeNil)

			Convey("Then the sink should receive all tuples", func() {
				So(si.len(), ShouldEqual, 5)
			})
		})

		Convey("When sending tuples from two sources", func() {
			for i := 0; i < 5; i++ {
				So(dsts[0].Write(ctx, t), ShouldBeNil)
				So(dsts[1].Write(ctx, t), ShouldBeNil)
			}
			si.Wait(10)
			srcs.stop(ctx)
			So(<-stopped, ShouldBeNil)

			Convey("Then the sink should receive all tuples", func() {
				So(si.len(), ShouldEqual, 10)
			})
		})

		Convey("When stopping sources", func() {
			srcs.stop(ctx)

			Convey("Then it should eventually stop", func() {
				for _, d := range dsts {
					d.waitUntilClosed()
					So(d.Write(ctx, t), ShouldPointTo, errPipeClosed)
				}
				So(<-stopped, ShouldBeNil)
			})
		})

		Convey("When stopping all inputs", func() {
			for _, d := range dsts {
				d.close()
			}
			Reset(func() {
				srcs.stop(ctx)
			})

			Convey("Then it shouldn't stop pouring", func() {
				// This actually doesn't guarantee anything, but test should
				// sometimes fail if the code above stopped the source.
				received := false
				select {
				case <-stopped:
					received = true
				default:
				}
				So(received, ShouldBeFalse)
			})
		})

		Convey("When stopping inputs after sending some tuples", func() {
			for i := 0; i < 5; i++ {
				So(dsts[0].Write(ctx, t), ShouldBeNil)
			}

			for i := 0; i < 3; i++ {
				So(dsts[1].Write(ctx, t), ShouldBeNil)
			}
			si.Wait(8)
			dsts[0].close()
			dsts[1].close()
			srcs.stop(ctx)
			So(<-stopped, ShouldBeNil)

			Convey("Then the sink should receive all tuples", func() {
				So(si.len(), ShouldEqual, 8)
			})
		})

		Convey("When adding a new input and sending a tuple", func() {
			r, s := newPipe("test3", 1)
			srcs.add("test_node_3", r)
			So(s.Write(ctx, t), ShouldBeNil)
			si.Wait(1)
			srcs.stop(ctx)
			So(<-stopped, ShouldBeNil)

			Convey("Then the sink should receive the tuple", func() {
				So(si.len(), ShouldEqual, 1)
			})
		})

		Convey("When adding a new input with the duplicated name", func() {
			r, _ := newPipe("test3", 1)
			err := srcs.add("test_node_1", r)

			Convey("Then it should fail", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When remove an input after sending a tuple", func() {
			So(dsts[0].Write(ctx, t), ShouldBeNil)
			srcs.enableGracefulStop()
			srcs.remove("test_node_1")

			Convey("Then the input should eventually be closed", func() {
				for {
					err := dsts[0].Write(ctx, t)
					if err != nil {
						So(err, ShouldPointTo, errPipeClosed)
						break
					}
				}
			})

			Convey("Then the sink should receive it", func() {
				srcs.stop(ctx)
				So(<-stopped, ShouldBeNil)
				So(si.len(), ShouldEqual, 1)
			})

			Convey("Then the other input should still work", func() {
				So(dsts[1].Write(ctx, t), ShouldBeNil)
				srcs.stop(ctx)
				So(<-stopped, ShouldBeNil)
				So(si.len(), ShouldEqual, 2)
			})
		})
	})
}

func TestDataSourcesFailure(t *testing.T) {
	Convey("Given a data source", t, func() {
		ctx := NewContext(nil)
		srcs := newDataSources(NTBox, "test_component")
		r, s := newPipe("test", 1)
		srcs.add("test_node", r)
		Reset(func() {
			s.close()
		})
		stopped := make(chan error, 1)
		go func() {
			stopped <- srcs.pour(ctx, WriterFunc(func(ctx *Context, t *Tuple) error {
				return errors.New("error")
			}), 4)
		}()
		srcs.state.Wait(TSRunning)
		Reset(func() {
			srcs.stop(ctx)
		})
		t := &Tuple{
			InputName: "some_component",
			Data: data.Map{
				"v": data.Int(1),
			},
		}

		Convey("When writing a tuple to it and the connected node returns an error", func() {
			So(s.Write(ctx, t), ShouldBeNil)
			srcs.stop(ctx)
			So(<-stopped, ShouldBeNil)

			Convey("Then numError should be increased", func() {
				So(srcs.numErrors, ShouldEqual, 1)
			})
		})
	})
}

func (s *pipeSender) waitUntilClosed() {
	for {
		s.rwm.RLock()
		if s.closed {
			s.rwm.RUnlock()
			return
		}
		s.rwm.RUnlock()
	}
}

// TODO: add fail tests of dataSources

func TestDataDestinations(t *testing.T) {
	ctx := NewContext(nil)

	Convey("Given an empty data destination", t, func() {
		dsts := newDataDestinations(NTBox, "test_component")
		t := &Tuple{
			InputName: "test_component",
			Data: data.Map{
				"v": data.Int(1),
			},
		}

		Convey("When sending a tuple", func() {
			var err error
			So(func() {
				err = dsts.Write(ctx, t)
			}, ShouldNotPanic)

			Convey("Then it shouldn't fail", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When getting nodeType it should be NTBox", func() {
			So(dsts.nodeType, ShouldEqual, NTBox)
		})
	})

	Convey("Given data destinations", t, func() {
		dsts := newDataDestinations(NTBox, "test_component")
		recvs := make([]*pipeReceiver, 2)
		for i := range recvs {
			r, s := newPipe(fmt.Sprint("test", i+1), 1)
			recvs[i] = r
			dsts.add(fmt.Sprint("test_node_", i+1), s)
		}
		t := &Tuple{
			InputName: "test_component",
			Data: data.Map{
				"v": data.Int(1),
			},
		}

		Convey("When sending a tuple", func() {
			So(dsts.Write(ctx, t), ShouldBeNil)

			Convey("Then all destinations should receive it", func() {
				t1, ok := <-recvs[0].in
				So(ok, ShouldBeTrue)
				t2, ok := <-recvs[1].in
				So(ok, ShouldBeTrue)

				Convey("And tuples should have the correct input name", func() {
					So(t1.InputName, ShouldEqual, "test1")
					So(t2.InputName, ShouldEqual, "test2")
				})
			})
		})

		Convey("When sending closing the destinations after sending a tuple", func() {
			So(dsts.Write(ctx, t), ShouldBeNil)
			So(dsts.Close(ctx), ShouldBeNil)

			Convey("Then all receiver should receive a closing signal after the tuple", func() {
				for _, r := range recvs {
					_, ok := <-r.in
					So(ok, ShouldBeTrue)
					_, ok = <-r.in
					So(ok, ShouldBeFalse)
				}
			})
		})

		Convey("When one destination is closed by the receiver side", func() {
			recvs[0].close()
			drainReceiver(recvs[0])
			Reset(func() {
				dsts.Close(ctx)
			})

			Convey("Then the destination receiver should eventually be removed", func() {
				go func() {
					for _ = range recvs[1].in {
					}
				}()
				for {
					if !dsts.has("test_node_1") {
						break
					}
					dsts.Write(ctx, t)
				}

				_, ok := <-recvs[0].in
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When adding a new destination after sending a tuple", func() {
			for _, r := range recvs {
				r := r
				go func() {
					for _ = range r.in {
					}
				}()
			}
			So(dsts.Write(ctx, t), ShouldBeNil)

			r, s := newPipe("test3", 1)
			So(dsts.add("test_node_3", s), ShouldBeNil)
			Reset(func() {
				dsts.Close(ctx)
			})

			Convey("Then the new receiver shouldn't receive the first tuple", func() {
				recved := false
				select {
				case <-r.in:
					recved = true
				default:
				}
				So(recved, ShouldBeFalse)
			})

			Convey("Then the new receiver should receive a new tuple", func() {
				So(dsts.Write(ctx, t), ShouldBeNil)
				_, ok := <-r.in
				So(ok, ShouldBeTrue)
			})
		})

		Convey("When adding a destination with the duplicated name", func() {
			_, s := newPipe("hoge", 1)
			err := dsts.add("test_node_1", s)

			Convey("Then it should fail", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When removing a destination", func() {
			dsts.remove("test_node_1")

			Convey("Then the destination should be closed", func() {
				_, ok := <-recvs[0].in
				So(ok, ShouldBeFalse)
			})

			Convey("Then Write should work", func() {
				So(dsts.Write(ctx, t), ShouldBeNil)
				_, ok := <-recvs[1].in
				So(ok, ShouldBeTrue)
			})
		})

		Convey("When removing a destination after sending a tuple", func() {
			go func() {
				for _ = range recvs[1].in {
				}
			}()
			Reset(func() {
				dsts.Close(ctx)
			})
			So(dsts.Write(ctx, t), ShouldBeNil)
			dsts.remove("test_node_1")

			Convey("Then the destination should be able to receive the tuple", func() {
				_, ok := <-recvs[0].in
				So(ok, ShouldBeTrue)
				_, ok = <-recvs[0].in
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When removing a nonexistent destination", func() {
			Convey("Then it shouldn't panic", func() {
				So(func() {
					dsts.remove("test_node_100")
				}, ShouldNotPanic)
			})
		})

		Convey("When pausing", func() {
			dsts.pause()
			ch := make(chan error)
			go func() {
				ch <- fmt.Errorf("dummy error")
				ch <- dsts.Write(ctx, t)
			}()
			<-ch

			Convey("Then the write should be blocked", func() {
				Reset(func() {
					dsts.resume()
					<-ch
				})

				blocked := true
				select {
				case <-ch:
					blocked = false
				default:
				}
				So(blocked, ShouldBeTrue)
			})

			Convey("Then resume method unblocks the write", func() {
				dsts.resume()
				So(<-ch, ShouldBeNil)
			})
		})
	})
}

func (d *dataDestinations) has(name string) bool {
	d.rwm.RLock()
	defer d.rwm.RUnlock()
	_, ok := d.dsts[name]
	return ok
}
