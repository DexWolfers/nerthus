
go-datastructures
=================

Go-datastructures is a collection of useful, performant, and threadsafe Go
datastructures.

### NOTE: only tested with Go 1.3+.
 

#### Queue

Package contains both a normal and priority queue.  Both implementations never
block on send and grow as much as necessary.  Both also only return errors if
you attempt to push to a disposed queue and will not panic like sending a
message on a closed channel.  The priority queue also allows you to place items
in priority order inside the queue.  If you give a useful hint to the regular
queue, it is actually faster than a channel.  The priority queue is somewhat
slow currently and targeted for an update to a Fibonacci heap.

Also included in the queue package is a MPMC threadsafe ring buffer. This is a
block full/empty queue, but will return a blocked thread if the queue is
disposed while a thread is blocked.  This can be used to synchronize goroutines
and ensure goroutines quit so objects can be GC'd.  Threadsafety is achieved
using only CAS operations making this queue quite fast.  Benchmarks can be found
in that package.
 

### Installation

 1. Install Go 1.3 or higher.
 2. Run `go get github.com/Workiva/go-datastructures/...`

### Updating

When new code is merged to master, you can use

	go get -u github.com/Workiva/go-datastructures/...

To retrieve the latest version of go-datastructures.

### Testing

To run all the unit tests use these commands:

	cd $GOPATH/src/github.com/Workiva/go-datastructures
	go get -t -u ./...
	go test ./...

Once you've done this once, you can simply use this command to run all unit tests:

	go test ./...


### Contributing

Requirements to commit here:

 - Branch off master, PR back to master.
 - `gofmt`'d code.
 - Compliance with [these guidelines](https://code.google.com/p/go-wiki/wiki/CodeReviewComments)
 - Unit test coverage
 - [Good commit messages](http://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html)


### Maintainers

 - Dustin Hiatt <[dustin.hiatt@workiva.com](mailto:dustin.hiatt@workiva.com)>
 - Alexander Campbell <[alexander.campbell@workiva.com](mailto:alexander.campbell@workiva.com)>


### Change 

change by Nerthus Team.