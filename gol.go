package main

import (
    "fmt"
    "strconv"
    "strings"
    "math"
)

type workerChans struct {

    disChan chan byte     // disChan - the channel which the cell data is sent to the worker along, and
                          //           which the cell data is sent back to the distributor through
    aboveSend chan<- byte // aboveSend - the channel where the halo for the worker above is sent
    aboveRec <-chan byte  // aboveRec - the channel where the halo from the worker above is received
    belowSend chan<- byte // belowSend - the channel where the halo for the worker below is sent
    belowRec <-chan byte  // belowRec - the channel where the halo from the worker below is received
    keyRec <-chan string  // keyRec - receives the user's keypress
}

// performs x mod y
// works correctly for negative x
// eg -1 mod 8 = 7
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

// n - the number assigned to the worker (used for debugging)
// height - the height of the slice the worker will be sent
// width - the width of the slice the worker will be sent
// turns - the number of turns to run the game for
func worker(n, height, width, turns int, wc workerChans) {

    // allocate two buffers to hold the cells
    strip := make([][]byte, height)
    buffStrip := make([][]byte, height)

    for i := range strip {
        strip[i] = make([]byte, width)
        buffStrip[i] = make([]byte, width)
    }

    // receive the cell data from the distributor
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            strip[y][x] = <-wc.disChan
        }
    }

    paused := false
    for turn := 0; turn < turns; turn++ {

        // send halos
        for haloX := 0; haloX < width; haloX++ {
            wc.aboveSend <- strip[1][haloX]
            wc.belowSend <- strip[height - 2][haloX]
        }

        // receive halos
        for haloX := 0; haloX < width; haloX++ {
            strip[0][haloX]          = <-wc.aboveRec
            strip[height - 1][haloX] = <-wc.belowRec
        }

        // for each cell (excluding the halo rows)
        for y := 1; y < height - 1; y++ {
            for x := 0; x < width; x++ {

                var sum int
                // + + +
                // + . + calculate the number of neighbours
                // + + +
                sum = int(strip[mod((y-1) ,height)][mod((x-1) ,width)]) + int(strip[mod((y-1), height)][mod((x), width)]) + int(strip[mod((y-1), height)][mod((x+1), width)]) +
                      int(strip[mod((y), height)][mod((x-1), width)])                        +                              int(strip[(y) % height][(x+1) % width])           +
                      int(strip[mod((y+1), height)][mod((x-1), width)]) +     int(strip[(y+1) % height][(x) % width])     + int(strip[(y+1) % height][(x+1) % width])
                // division by 255 because an alive cell is stored as 255 in
                // the image file
                sum /= 255

                // game of life logic
                if strip[y][x] == 255 && sum < 2 {
                    buffStrip[y][x] = 0
                } else if strip[y][x] == 255 && sum > 3 {
                    buffStrip[y][x] = 0
                } else if strip[y][x] == 0 && sum == 3 {
                    buffStrip[y][x] = 255
                } else if strip[y][x] == 255 && (sum == 3 || sum == 2) {
                    buffStrip[y][x] = strip[y][x]
                } else {
                    buffStrip[y][x] = 0
                }
            }
        }

        // swap the pointers
        strip, buffStrip = buffStrip, strip

        select {
        case c := <-wc.keyRec:
            if c == "p" {
                if paused {
                    paused = false
                    fmt.Println("Continuing")
                } else {
                    paused = true
                    fmt.Printf("The current turn is %d\n", turn + 1)

                    for paused {
                        select {
                        case c := <-wc.keyRec:
                            if c == "s" {
                                fmt.Println("Pressed S")

                            } else if c == "p" {
                                fmt.Println("Continuing")
                                paused = false

                            } else if c == "q" {
                                fmt.Println("Pressed Q")
                            }
                        }
                    }
                }
            } else if c == "s" {
                fmt.Println("Pressed S")
            } else if c == "q" {
                fmt.Println("Pressed Q")
            }
        default:
        }
    }

    // send the cell data back to the distributor after all the turns have been
    // completed
    for y := 1; y < height - 1; y++ {
        for x := 0; x < width; x++ {
            wc.disChan <- strip[y][x]
        }
    }
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell) {

    // Create the 2D slice to store the world.
    world := make([][]byte, p.imageHeight)
    for i := range world {
        world[i] = make([]byte, p.imageWidth)
    }

    // Request the io goroutine to read in the image with the given filename.
    d.io.command <- ioInput
    d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

    // The io goroutine sends the requested image byte by byte, in rows.
    for y := 0; y < p.imageHeight; y++ {
        for x := 0; x < p.imageWidth; x++ {
            val := <-d.io.inputVal
            if val != 0 {
                world[y][x] = val
            }
        }
    }

    // create a channels for the halo exchange
    disChans := make([]chan byte, p.threads)
    sendChans := make([]chan<- byte, 2 * p.threads)
    recChans := make([]<-chan byte, 2 * p.threads)
    keyChans := make([]chan string, p.threads)

    for i := 0; i < 2 * p.threads; i++ {
        c := make(chan byte, p.imageWidth)
        sendChans[i] = c
        recChans[i] = c
    }

    go getKeyboardCommand(keyChans)

    //sending data to the workers
    for i := 0; i < p.threads; i++ {
        disChans[i] = make(chan byte)

        k := make(chan string)
        keyChans[i] = k

        lowerBound := int(math.Round(float64(p.imageHeight * i) / float64(p.threads)))
        upperBound := int(math.Round((float64(p.imageHeight * (i + 1)) / float64(p.threads))))

        // assign the correct channels to the worker
        var wc workerChans
        wc.disChan = disChans[i]
        wc.aboveSend = sendChans[i * 2]
        wc.aboveRec = recChans[mod((i * 2) - 1, 2 * p.threads)]
        wc.belowSend = sendChans[(i * 2) + 1]
        wc.belowRec = recChans[mod((i + 1) * 2, 2 * p.threads)]
        wc.keyRec = keyChans[i]

        go worker(i, int(upperBound - lowerBound) + 2, p.imageWidth, p.turns, wc)

        for y := lowerBound - 1; y < upperBound + 1; y++ {
            for x := 0; x < p.imageWidth; x++ {
                disChans[i] <- world[mod(y, p.imageHeight)][x]
            }
        }
    }

    // the magic happens
    // it actually does

    //receiving data from the workers and reconstructing
    for i := 0; i < p.threads; i++ {

        lowerBound := math.Round(float64(p.imageHeight * i) / float64(p.threads))
        upperBound := math.Round((float64(p.imageHeight * (i + 1)) / float64(p.threads)))

        for y := lowerBound; y < upperBound; y++ {
            for x := 0; x < p.imageWidth; x++ {
                world[int(y)][x] = <-disChans[i]
            }
        }
    }

    // kind of bad, but idk
    // we have to have the second select statement because we need a select
    // statement without a default case to stop the busy waiting.

    //     fmt.Println("sending")
    //pressing s prints the current state of the board out to a file
    // pressing p pauses execution, pressing p again unpauses
    // pressing q does something.......
    // select {
    // case ascii_value := <-d.key:
    //     c := string(ascii_value)

    //     if c == "s" {
    //         fmt.Println("Pressed S")
    //         sPressed(p, d, world, turns)

    //     } else if c == "p" {
    //         if paused {
    //             fmt.Println("Continuing")
    //             paused = false
    //         } else {
    //             fmt.Printf("The current turn is %d\n", turns + 1)
    //             paused = true

    //             for paused && !needToStop {
    //                 select {
    //                 case ascii_value := <-d.key:
    //                     c := string(ascii_value)
    //                     if c == "s" {
    //                         fmt.Println("Pressed S")
    //                         sPressed(p, d, world, turns)

    //                     } else if c == "p" {
    //                         fmt.Println("Continuing")
    //                         paused = false
    //                     } else if c == "q" {
    //                         fmt.Println("Pressed Q")
    //                         sPressed(p, d, world, turns)
    //                         needToStop = true
    //                     }
    //                 }
    //             }
    //         }
    //     } else if c == "q" {
    //         fmt.Println("Pressed Q")
    //         sPressed(p, d, world, turns)
    //         needToStop = true
    //     }

    // default:
    // }

    // if !paused {
    //     select {
    //     case <-d.timer:
    //         // count alive cells
    //         var sum int
    //         for y := 0; y < p.imageHeight; y++ {
    //             for x := 0; x < p.imageWidth; x++ {
    //                 if world[y][x] == 255 {
    //                     sum += 1
    //                 }
    //             }
    //         }
    //         fmt.Printf("There are currently %d cells alive\n", sum)
    //     default:
    //     }
    // }

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
