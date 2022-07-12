package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Outcome int

const (
	success Outcome = iota
	fail
	timeout
)

var outcomeCh = make(chan Outcome, 3)

const outcomeTimeOutInterval = time.Millisecond * 100

func timer(t time.Duration, wg *sync.WaitGroup) {
	done := make(chan struct{})

	go func() {
		time.Sleep(t)
		close(done)
	}()

	counter := 0

	go func() {
		defer wg.Done()
	outer:
		for {
			select {
			case <-done:
				break outer
			default:
				time.Sleep(time.Millisecond * 50)
				counter++
				fmt.Printf("%vms has passed\n", counter*50)
			}
		}
	}()
}

func request() (int, error) {
	switch <-outcomeCh {
	case success:
		println("A request was successful")
		return 200, nil
	case fail:
		println("A request failed")
		return -1, errors.New("request failed")
	case timeout:
		time.Sleep(outcomeTimeOutInterval)
		println("A timeout detected at request()")
		return -1, errors.New("request timeout")
	default:
		return -1, errors.New("unexpected error happened")
	}
}

type AOut int
type BOut int
type COut int
type AIn int
type BIn int

type Input struct {
	a int
	b int
}

type CIn struct {
	retA AOut
	retB BOut
}

type processor struct {
	outA chan AOut
	outB chan BOut
	outC chan COut
	inC  chan CIn
	errs chan error
}

func GatherAndProcess(ctx context.Context, data Input) (COut, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	p := processor{
		outA: make(chan AOut, 1),
		outB: make(chan BOut, 1),
		inC:  make(chan CIn, 1),
		outC: make(chan COut, 1),
		errs: make(chan error, 2),
	}
	p.launch(ctx, data)
	inputC, err := p.waitForAB(ctx)
	if err != nil {
		return COut(-1), err
	}
	p.inC <- inputC
	out, err := p.waitForC(ctx)
	return out, err
}

func (p processor) launch(ctx context.Context, data Input) {
	// requesting AOut
	go func() {
		ret, err := request()
		if err != nil {
			// NOTE: no value should be returned on error
			// otherwise, the elegant selecting on error and output
			// in waitforAB shall be redesigned
			p.errs <- err
			return
		}
		p.outA <- AOut(ret)
	}()

	// requesting BOut
	go func() {
		ret, err := request()
		if err != nil {
			p.errs <- err
			return
		}
		p.outB <- BOut(ret)
	}()

	go func() {
		select {
		case <-p.inC: // pretending using inC to make request
			ret, err := request()
			if err != nil {
				p.errs <- err
				return
			}
			p.outC <- COut(ret)
		case <-ctx.Done():

		}
	}()
}

func (p processor) waitForAB(ctx context.Context) (CIn, error) {
	ret := CIn{}
	for i := 0; i < 2; {
		select {
		case <-ctx.Done():
			return CIn{}, errors.New("Timeout(ctx) when waiting for AB")
		case err := <-p.errs:
			return CIn{}, fmt.Errorf("waitForAB got an error: %w", err)
		case ret.retA = <-p.outA:
			println("retA received at waitforAB")
			i++
		case ret.retB = <-p.outB:
			println("retB received at waitforAB")
			i++
		}
	}
	return ret, nil
}

func (p processor) waitForC(ctx context.Context) (ret COut, err error) {
	select {
	case ret = <-p.outC:
		return ret, err
	case err = <-p.errs:
		return ret, fmt.Errorf("error detected at waitForC: %w", err)
	case <-ctx.Done():
		return ret, errors.New("timeout(ctx) when waiting for C")
	}
}

func main() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	outcomeCh <- success
	outcomeCh <- timeout
	outcomeCh <- success
	timer(200*time.Millisecond, &wg)

	ret, err := GatherAndProcess(context.Background(), Input{a: 114, b: 514})
	if err != nil {
		fmt.Printf("error received at main: %v\n", err)
	} else {
		fmt.Printf("Success, ret = %v\n", ret)
	}

	wg.Wait()
}
