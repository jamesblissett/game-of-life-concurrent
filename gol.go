package main

import (
	//"fmt"
	"strconv"
	"strings"
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell) {

	// Create the 2D slice to store the world.
	world := make([][]byte, p.imageHeight)
	buffWorld := make([][]byte, p.imageHeight)
//	var tempWorld [][]byte

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
		}
	}

	// Calculate the new state of Game of Life after the given number of turns.
	for turns := 0; turns < p.turns; turns++ {
		for y := 0; y < p.imageHeight; y++ {
			for x := 0; x < p.imageWidth; x++ {
				var sum int
				sum =   int(world[mod((y-1) ,p.imageHeight)][mod((x-1) ,p.imageWidth)]) + int(world[mod((y-1), p.imageHeight)][mod((x), p.imageWidth)]) + int(world[mod((y-1), p.imageHeight)][mod((x+1), p.imageWidth)]) +
							  int(world[mod((y), p.imageHeight)][mod((x-1), p.imageWidth)])	                     +                                int(world[(y) % p.imageHeight][(x+1) % p.imageWidth])   +
							  int(world[mod((y+1), p.imageHeight)][mod((x-1), p.imageWidth)]) + int(world[(y+1) % p.imageHeight][(x) % p.imageWidth]) + int(world[(y+1) % p.imageHeight][(x+1) % p.imageWidth])
				sum /= 255
				if world[y][x] == 255 && sum < 2 {
					buffWorld[y][x] = 0
				} else if world[y][x] == 255 && sum > 3 {
					buffWorld[y][x] = 0
				} else if world[y][x] == 0 && sum == 3{
					buffWorld[y][x] = 255
				} else if world[y][x] == 255 && (sum == 3 || sum == 2){
					buffWorld[y][x] = world[y][x]
				} else {
					buffWorld[y][x] = 0
				}
			}
		}
		//swaps pointers
		world, buffWorld = buffWorld, world
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
