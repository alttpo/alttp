package taskqueue

import "sync"

type WorkerFunc[T any] func(*Q[T], T)

type Q[T any] struct {
	c      chan T
	wg     sync.WaitGroup
	worker WorkerFunc[T]
}

func NewQ[T any](workerCount int, worker WorkerFunc[T]) (q *Q[T]) {
	if worker == nil {
		panic("worker cannot be nil")
	}
	if workerCount <= 0 {
		panic("workerCount must be at least 1")
	}

	q = &Q[T]{
		c:      make(chan T, workerCount),
		worker: worker,
	}

	for n := 0; n < workerCount; n++ {
		go func() {
			for item := range q.c {
				q.runWorker(item)
			}
		}()
	}

	return
}

func (q *Q[T]) runWorker(item T) {
	defer q.wg.Done()
	q.worker(q, item)
}

func (q *Q[T]) SubmitItem(item T) {
	q.wg.Add(1)
	q.c <- item
}

func (q *Q[T]) Wait() {
	q.wg.Wait()
}

func (q *Q[T]) Close() {
	close(q.c)
}
