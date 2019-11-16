package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nsf/termbox-go"
)

func mod(x int, y int) int {
	m := x % y
	if x < 0 && y < 0{
		m -= y
	}
	if x < 0 && y > 0{
		m += y
	}

	return m
}

func worker(height, width int, c chan byte){
	// fmt.Println(height)
	// fmt.Println(width)

	Strip := make([][]byte, height)
	buffStrip := make([][]byte, height)

	for i := range Strip {
		Strip[i] = make([]byte, width)
		buffStrip[i] = make([]byte, width)
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			Strip[y][x] = <-c
			//testing
			//buffStrip[y][x] = 128
		}
	}

	for y := 1; y < height - 1; y++ {
		for x := 0; x < width; x++ {
			var sum int
			sum =   int(Strip[mod((y-1) ,height)][mod((x-1) ,width)]) + int(Strip[mod((y-1), height)][mod((x), width)]) + int(Strip[mod((y-1), height)][mod((x+1), width)]) +
							int(Strip[mod((y), height)][mod((x-1), width)])	                     +                                int(Strip[(y) % height][(x+1) % width])   +
							int(Strip[mod((y+1), height)][mod((x-1), width)]) + int(Strip[(y+1) % height][(x) % width]) + int(Strip[(y+1) % height][(x+1) % width])
			sum /= 255
			if Strip[y][x] == 255 && sum < 2 {
				buffStrip[y][x] = 0
			} else if Strip[y][x] == 255 && sum > 3 {
				buffStrip[y][x] = 0
			} else if Strip[y][x] == 0 && sum == 3{
				buffStrip[y][x] = 255
			} else if Strip[y][x] == 255 && (sum == 3 || sum == 2){
				buffStrip[y][x] = Strip[y][x]
			} else {
				buffStrip[y][x] = 0
			}
		}
	}

	for y := 1; y < height - 1; y++ {
		for x := 0; x < width; x++ {
			c <- buffStrip[y][x]
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell) {
	//fmt.Println(p.threads)
	// Create the 2D slice to store the world.
	world := make([][]byte, p.imageHeight)
	buffWorld := make([][]byte, p.imageHeight)

	paused := false


	for i := range world {
		world[i] = make([]byte, p.imageWidth)
		buffWorld[i] = make([]byte, p.imageWidth)
	}

	// Request the io goroutine to read in the image with the given filename.
	d.io.command <- ioInput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

	// The io goroutine sends the requested image byte by byte, in rows.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			val := <-d.io.inputVal
			if val != 0 {
				//fmt.Println("Alive cell at", x, y)
				world[y][x] = val
			}
			//testing. Grey if cell not processed
			//buffWorld[y][x] = 128
		}
	}

	// Calculate the new state of Game of Life after the given number of turns.
	for turns := 0; turns < p.turns; turns++ {
		chans := make([]chan byte, p.threads)

		//sending data to the workers
		for i := 0; i < p.threads; i++ {
			chans[i] = make(chan byte)
			go worker(p.imageHeight / p.threads + 2, p.imageWidth, chans[i])
			for y := ((p.imageHeight * i) / p.threads) - 1; y < ((p.imageHeight * (i+1)) / p.threads) + 1; y++ {
				for x := 0; x < p.imageWidth; x++ {
					chans[i] <- world[mod(y, p.imageHeight)][x]
				}
			}
		}


		//receiving data from the workers and reconstructing
		for i := 0; i < p.threads; i++ {
			for y := (p.imageHeight * i) / p.threads; y < (p.imageHeight * (i+1)) / p.threads; y++ {
				for x := 0; x < p.imageWidth; x++ {
					buffWorld[y][x] = <-chans[i]
				}
			}
		}
		//fmt.Println("done img")
		//swaps pointers
		world, buffWorld = buffWorld, world

		// kind of bad, but idk
		// we have to have the second select statement because we need a select
		// statement without a default case to stop the busy waiting.

		// pressing s prints the current state of the board out to a file
		// pressing p pauses execution, pressing p again unpauses
		// pressing q does something.......
		select {
		case ascii_value := <-d.key:
			c := string(ascii_value)

			if c == "s" {
				fmt.Println("Pressed S")
				sPressed(p, d, world, turns)

			} else if c == "p" {
				if paused {
					fmt.Println("Continuing")
					paused = false
				} else {
					fmt.Printf("The current turn is %d\n", turns + 1)
					paused = true

					for paused {
						select {
						case ascii_value := <-d.key:
							c := string(ascii_value)
							if c == "s" {
								fmt.Println("Pressed S")
								sPressed(p, d, world, turns)

							} else if c == "p" {
								fmt.Println("Continuing")
								paused = false
							} else if c == "q" {
								fmt.Println("Pressed Q")
								qPressed()
							}
						}
					}
				}
			} else if c == "q" {
				fmt.Println("Pressed Q")
				qPressed()
			}

		default:
		}
	}

	// Request the io goroutine to output the image with the given filename.
	d.io.command <- ioOutput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")


	// The io goroutine sends the requested image byte by byte, in rows.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			d.io.outputVal <- world[y][x]
		}
	}




	// Create an empty slice to store coordinates of cells that are still alive after p.turns are done.
	var finalAlive []cell
	// Go through the world and append the cells that are still alive.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0 {
				finalAlive = append(finalAlive, cell{x: x, y: y})
			}
		}
	}

	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle

	// Return the coordinates of cells that are still alive.
	alive <- finalAlive
}

// n is value to append to filename as the turn number
func sPressed(p golParams, d distributorChans, world [][]byte, n int) {
	d.io.command <- ioOutput
	d.io.filename <- strconv.Itoa(p.imageWidth) + "x" + strconv.Itoa(p.imageHeight) + "t" + strconv.Itoa(n)

	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			d.io.outputVal <- world[y][x]
		}
	}
}

func qPressed() {
	termbox.Close()
	os.Exit(0)
}
