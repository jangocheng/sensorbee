package bql

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/sensorbee/sensorbee.v0/core"
	"gopkg.in/sensorbee/sensorbee.v0/data"
)

type testFileWriter struct {
	m   sync.Mutex
	c   *sync.Cond
	cnt int
	tss []time.Time
}

func (w *testFileWriter) Write(ctx *core.Context, t *core.Tuple) error {
	w.m.Lock()
	defer w.m.Unlock()
	w.cnt++
	w.tss = append(w.tss, t.Timestamp)
	w.c.Broadcast()
	return nil
}

func (w *testFileWriter) wait(n int) {
	w.m.Lock()
	defer w.m.Unlock()
	for w.cnt < n {
		w.c.Wait()
	}
}

func TestFileSource(t *testing.T) {
	f, err := ioutil.TempFile("", "sbtest_bql_file_source")
	if err != nil {
		t.Fatal("Cannot create a temp file:", err)
	}
	name := f.Name()
	defer func() {
		os.Remove(name)
	}()
	now := time.Now()
	nowTs := data.Timestamp(now)

	// an empty line is intentionally included
	_, err = io.WriteString(f, fmt.Sprintf(`{"int":1, "ts":%v}

 {"int":2, "ts":%v}
  {"int":3, "ts":%v} `, nowTs, nowTs, nowTs))
	f.Close()
	if err != nil {
		t.Fatal("Cannot write to the temp file:", err)
	}

	Convey("Given a file", t, func() {
		ctx := core.NewContext(nil)
		params := data.Map{"path": data.String(name)}
		w := &testFileWriter{}
		w.c = sync.NewCond(&w.m)

		Convey("When reading the file by file source with default params", func() {
			s, err := createFileSource(ctx, &IOParams{}, params)
			So(err, ShouldBeNil)
			Reset(func() {
				s.Stop(ctx)
			})

			err = s.GenerateStream(ctx, w)
			So(err, ShouldBeNil)

			Convey("Then it should emit all tuples", func() {
				So(w.cnt, ShouldEqual, 3)
			})
		})

		Convey("When reading the file with custom timestamp field", func() {
			params["timestamp_field"] = data.String("ts")
			s, err := createFileSource(ctx, &IOParams{}, params)
			So(err, ShouldBeNil)
			Reset(func() {
				s.Stop(ctx)
			})

			err = s.GenerateStream(ctx, w)
			So(err, ShouldBeNil)

			Convey("Then it should emit all tuples", func() {
				So(w.cnt, ShouldEqual, 3)
			})

			Convey("Then it should have custom timestamps", func() {
				So(w.tss, ShouldHaveLength, w.cnt)
				for _, ts := range w.tss {
					So(ts, ShouldHappenOnOrBetween, now, now)
				}
			})
		})

		Convey("When reading the file with rewindable", func() {
			params["rewindable"] = data.True
			s, err := createFileSource(ctx, &IOParams{}, params)
			So(err, ShouldBeNil)
			Reset(func() {
				s.Stop(ctx)
			})

			ch := make(chan error, 1)
			go func() {
				ch <- s.GenerateStream(ctx, w)
			}()

			Convey("Then it should write all tuples and pause", func() {
				w.wait(3)
				So(w.cnt, ShouldEqual, 3)
				select {
				case <-ch:
					So("The source should not have stopped yet", ShouldBeNil)
				default:
				}
			})

			Convey("Then it should be able to rewind", func() {
				w.wait(3)
				rs := s.(core.RewindableSource)
				So(rs.Rewind(ctx), ShouldBeNil)
				w.wait(6)
				So(w.cnt, ShouldEqual, 6)
				select {
				case <-ch:
					So("The source should not have stopped yet", ShouldBeNil)
				default:
				}
			})

			Convey("Then it should be able to stop", func() {
				So(s.Stop(ctx), ShouldBeNil)
				err := <-ch
				So(err, ShouldBeNil)
			})
		})

		Convey("When reading the file with a repeat parameter", func() {
			params["repeat"] = data.Int(3)
			s, err := createFileSource(ctx, &IOParams{}, params)
			So(err, ShouldBeNil)
			Reset(func() {
				s.Stop(ctx)
			})

			err = s.GenerateStream(ctx, w)
			So(err, ShouldBeNil)

			Convey("Then it should emit all tuples", func() {
				// The source emits 3 tuples for 4 times including the first run.
				So(w.cnt, ShouldEqual, 12)
			})
		})

		Convey("When reading the file with a negative repeat parameter", func() {
			params["repeat"] = data.Int(-1)
			s, err := createFileSource(ctx, &IOParams{}, params)
			So(err, ShouldBeNil)
			Reset(func() {
				s.Stop(ctx)
			})

			ch := make(chan error, 1)
			go func() {
				ch <- s.GenerateStream(ctx, w)
			}()

			Convey("Then it should infinitely emit tuples", func() {
				w.wait(100)
				So(w.cnt, ShouldBeGreaterThanOrEqualTo, 100)
				select {
				case <-ch:
					So("The source should not have stopped yet", ShouldBeNil)
				default:
				}
			})

			Convey("Then it should be able to stop", func() {
				So(s.Stop(ctx), ShouldBeNil)
				err := <-ch
				So(err, ShouldBeNil)
			})
		})

		Convey("When reading the file with an interval parameter", func() {
			params["interval"] = data.Float(0.0001) // assuming this number is big enough
			s, err := createFileSource(ctx, &IOParams{}, params)
			So(err, ShouldBeNil)
			Reset(func() {
				s.Stop(ctx)
			})

			err = s.GenerateStream(ctx, w)
			So(err, ShouldBeNil)

			Convey("Then it should emit all tuples", func() {
				So(w.cnt, ShouldEqual, 3)
			})

			Convey("Then tuples' timestamps should have proper intervals", func() {
				So(w.tss, ShouldHaveLength, w.cnt)
				for i := 1; i < len(w.tss); i++ {
					So(w.tss[i], ShouldHappenOnOrAfter, w.tss[i-1].Add(100*time.Microsecond))
				}
			})
		})

		Convey("When creating a file source with invalid parameters", func() {
			Convey("Then missing path parameter should result in an error", func() {
				delete(params, "path")
				_, err := createFileSource(ctx, &IOParams{}, params)
				So(err, ShouldNotBeNil)
			})

			Convey("Then ill-formed timestamp_path should result in an error", func() {
				params["timestamp_field"] = data.String("/this/isnt/a/xpath")
				_, err := createFileSource(ctx, &IOParams{}, params)
				So(err, ShouldNotBeNil)
			})

			Convey("Then invalid timestamp_path value should result in an error", func() {
				params["timestamp_field"] = data.True
				_, err := createFileSource(ctx, &IOParams{}, params)
				So(err, ShouldNotBeNil)
			})

			Convey("Then invalid rewindable value should result in an error", func() {
				params["rewindable"] = data.Int(1)
				_, err := createFileSource(ctx, &IOParams{}, params)
				So(err, ShouldNotBeNil)
			})

			Convey("Then invalid repeat value should result in an error", func() {
				params["repeat"] = data.Float(1.5)
				_, err := createFileSource(ctx, &IOParams{}, params)
				So(err, ShouldNotBeNil)
			})

			Convey("Then invalid interval value should result in an error", func() {
				params["interval"] = data.Map{}
				_, err := createFileSource(ctx, &IOParams{}, params)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestFileSink(t *testing.T) {
	ctx := core.NewContext(nil)
	ioParams := &IOParams{}
	Convey("Given a temp directory path", t, func() {
		tdir, err := ioutil.TempDir("", "test_sb_file_sink")
		So(err, ShouldBeNil)
		Reset(func() {
			os.RemoveAll(tdir)
		})
		Convey("When create file sink without path", func() {
			params := data.Map{
				"truncate": data.False,
			}
			_, err := createFileSink(ctx, ioParams, params)
			Convey("Then the sink should not be created", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When create file sink with required param", func() {
			fn := filepath.Join(tdir, "file_sink.jsonl")
			params := data.Map{
				"path": data.String(fn),
			}
			si, err := createFileSink(ctx, ioParams, params)
			So(err, ShouldBeNil)
			Reset(func() {
				si.Close(ctx)
			})
			_, err = os.Stat(fn)
			So(err, ShouldBeNil)
			Convey("And when write a tuple to the sink", func() {
				d := data.Map{"k": data.Int(-1)}
				tu := core.NewTuple(d)
				So(si.Write(ctx, tu), ShouldBeNil)
				Convey("Then the tuple should be written in the file", func() {
					actualByte, err := ioutil.ReadFile(fn)
					So(err, ShouldBeNil)
					So(string(actualByte), ShouldEqual, `{"k":-1}
`)
				})
			})
		})

		Convey("When create file sink with truncate flag", func() {
			fn := filepath.Join(tdir, "file_sink2.jsonl")
			So(ioutil.WriteFile(fn, []byte(`{"k":-2}
`), 0644), ShouldBeNil)
			params := data.Map{
				"path":     data.String(fn),
				"truncate": data.True,
			}
			si, err := createFileSink(ctx, ioParams, params)
			So(err, ShouldBeNil)
			Reset(func() {
				si.Close(ctx)
			})
			Convey("And when write a tuple to the sink", func() {
				d := data.Map{"k": data.Int(-1)}
				tu := core.NewTuple(d)
				So(si.Write(ctx, tu), ShouldBeNil)
				Convey("Then the tuple should be written in the file", func() {
					actualByte, err := ioutil.ReadFile(fn)
					So(err, ShouldBeNil)
					So(string(actualByte), ShouldEqual, `{"k":-1}
`)
				})
			})
		})

		Convey("When create file sink with rotate option", func() {
			fn := filepath.Join(tdir, "file_sink3.jsonl")
			params := data.Map{
				"path":     data.String(fn),
				"max_size": data.Int(10),
			}
			si, err := createFileSink(ctx, ioParams, params)
			So(err, ShouldBeNil)
			Reset(func() {
				si.Close(ctx)
			})
			Convey("Then the sink is created as lumberjack object", func() {
				ws, ok := si.(*writerSink)
				So(ok, ShouldBeTrue)
				_, ok = ws.w.(*lumberjack.Logger)
				So(ok, ShouldBeTrue)

				Convey("And when write a tuple to the sink", func() {
					d := data.Map{"k": data.Int(-1)}
					tu := core.NewTuple(d)
					So(si.Write(ctx, tu), ShouldBeNil)
					Convey("Then the tuple should be written in the file", func() {
						actualByte, err := ioutil.ReadFile(fn)
						So(err, ShouldBeNil)
						So(string(actualByte), ShouldEqual, `{"k":-1}
`)
					})
				})
			})
		})

		Convey("When create file sink with rotate option and truncate (but file is empty)", func() {
			fn := filepath.Join(tdir, "file_sink4.jsonl")
			params := data.Map{
				"path":     data.String(fn),
				"max_size": data.Int(10),
				"truncate": data.True,
			}
			si, err := createFileSink(ctx, ioParams, params)
			So(err, ShouldBeNil)
			Reset(func() {
				si.Close(ctx)
			})
			Convey("Then the sink is created as lumberjack object", func() {
				ws, ok := si.(*writerSink)
				So(ok, ShouldBeTrue)
				_, ok = ws.w.(*lumberjack.Logger)
				So(ok, ShouldBeTrue)

				Convey("And when write a tuple to the sink", func() {
					d := data.Map{"k": data.Int(-1)}
					tu := core.NewTuple(d)
					So(si.Write(ctx, tu), ShouldBeNil)
					Convey("Then the tuple should be written in the file", func() {
						actualByte, err := ioutil.ReadFile(fn)
						So(err, ShouldBeNil)
						So(string(actualByte), ShouldEqual, `{"k":-1}
`)
					})
				})
			})
		})

		Convey("When create file sink with rotate option and truncated", func() {
			fn := filepath.Join(tdir, "file_sink5.jsonl")
			So(ioutil.WriteFile(fn, []byte(`{"k":-2}
`), 0644), ShouldBeNil)
			params := data.Map{
				"path":     data.String(fn),
				"max_size": data.Int(10),
				"truncate": data.True,
			}
			si, err := createFileSink(ctx, ioParams, params)
			So(err, ShouldBeNil)
			Reset(func() {
				si.Close(ctx)
			})
			Convey("Then the sink is created as lumberjack object", func() {
				ws, ok := si.(*writerSink)
				So(ok, ShouldBeTrue)
				_, ok = ws.w.(*lumberjack.Logger)
				So(ok, ShouldBeTrue)

				Convey("And when write a tuple to the sink", func() {
					d := data.Map{"k": data.Int(-1)}
					tu := core.NewTuple(d)
					So(si.Write(ctx, tu), ShouldBeNil)
					Convey("Then the tuple should be written in the file", func() {
						actualByte, err := ioutil.ReadFile(fn)
						So(err, ShouldBeNil)
						So(string(actualByte), ShouldEqual, `{"k":-1}
`)
					})
				})
			})
		})
	})
}
