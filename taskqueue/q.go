package taskqueue

import "sync"

type WorkerFunc[T any] func(*Q[T], T)

type I[T any] struct {
	job  WorkerFunc[T]
	item T
}

type Q[T any] struct {
	c      chan I[T]
	wg     sync.WaitGroup
	worker WorkerFunc[T]
}

func NewQ[T any](workerCount int, chanSize int, worker WorkerFunc[T]) (q *Q[T]) {
	if worker == nil {
		panic("worker cannot be nil")
	}
	if workerCount <= 0 {
		panic("workerCount must be at least 1")
	}

	q = &Q[T]{
		c:      make(chan I[T], chanSize),
		worker: worker,
	}

	for n := 0; n < workerCount; n++ {
		go func() {
			for i := range q.c {
				q.runJob(i)
			}
		}()
	}

	return
}

func (q *Q[T]) runJob(i I[T]) {
	defer q.wg.Done()
	i.job(q, i.item)
}

func (q *Q[T]) SubmitItem(item T) {
	q.wg.Add(1)
	q.c <- I[T]{q.worker, item}
}

func (q *Q[T]) SubmitJob(item T, job WorkerFunc[T]) {
	q.wg.Add(1)
	q.c <- I[T]{job, item}
}

func (q *Q[T]) Wait() {
	q.wg.Wait()
}

func (q *Q[T]) Close() {
	close(q.c)
}
