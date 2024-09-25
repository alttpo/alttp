package taskqueue

import (
	"fmt"
	"runtime/debug"
	"sync"
)

type WorkerFunc[T any] func(*Q[T], T)

type I[T any] struct {
	worker WorkerFunc[T]
	item   T
}

type Q[T any] struct {
	c  chan I[T]
	wg sync.WaitGroup
}

func NewQ[T any](workerCount int, chanSize int) (q *Q[T]) {
	if workerCount <= 0 {
		panic("workerCount must be at least 1")
	}

	q = &Q[T]{
		c: make(chan I[T], chanSize),
	}

	for n := 0; n < workerCount; n++ {
		go func() {
			for i := range q.c {
				q.runWorker(i)
			}
		}()
	}

	return
}

func (q *Q[T]) runWorker(i I[T]) {
	defer func() {
		if ex := recover(); ex != nil {
			fmt.Printf("taskqueue: RECOVER: %v\n%s\n", ex, string(debug.Stack()))
		}
		q.wg.Done()
	}()

	i.worker(q, i.item)
}

func (q *Q[T]) SubmitTask(item T, worker WorkerFunc[T]) {
	q.wg.Add(1)
	q.c <- I[T]{worker, item}
}

func (q *Q[T]) Wait() {
	q.wg.Wait()
}

func (q *Q[T]) Close() {
	close(q.c)
}
