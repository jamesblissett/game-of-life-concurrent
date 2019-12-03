package main

import (
    "fmt"
    "math"
    "strconv"
    "strings"
    "time"
)

type workerChans struct {
    disChan chan byte // disChan - the channel which the cell data is sent to the worker along, and
    //           which the cell data is sent back to the distributor through
    aboveSend chan<- byte // aboveSend - the channel where the halo for the worker above is sent
    aboveRec  <-chan byte // aboveRec - the channel where the halo from the worker above is received
    belowSend chan<- byte // belowSend - the channel where the halo for the worker below is sent
    belowRec  <-chan byte // belowRec - the channel where the halo from the worker below is received
    outSlice  <-chan bool // to tell the worker to output its slice
}

// performs x mod y
// works correctly for negative x
// eg -1 mod 8 = 7
func mod(x int, y int) int {
    m := x % y
    if x < 0 && y < 0 {
        m -= y
    }
    if x < 0 && y > 0 {
        m += y
    }
    return m
}

// n - the number assigned to the worker (used for debugging)
// height - the height of the slice the worker will be sent
// width - the width of the slice the worker will be sent
// turns - the number of turns to run the game for
func worker(n, height, width, turns int, wc workerChans, tickChan chan bool) {

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

    cond := true
    for turn := 0; turn < turns; turn++ {
        //fmt.Printf("The current turn is %d, %d\n", turn, n)

        cond = true
        for cond {
            select {
            case _ = <-wc.outSlice:
                // send the cell data back to the distributor after all the turns have been
                // completed
                for y := 1; y < height-1; y++ {
                    for x := 0; x < width; x++ {
                        wc.disChan <- strip[y][x]
                    }
                }
            case _ = <-tickChan:
                cond = false
            }
        }

        // send halos
        for haloX := 0; haloX < width; haloX++ {
            wc.aboveSend <- strip[1][haloX]
            wc.belowSend <- strip[height-2][haloX]
        }

        // receive halos
        for haloX := 0; haloX < width; haloX++ {
            strip[0][haloX] = <-wc.aboveRec
            strip[height-1][haloX] = <-wc.belowRec
        }

        // for each cell (excluding the halo rows)
        for y := 1; y < height-1; y++ {
            for x := 0; x < width; x++ {

                var sum int
                // + + +
                // + . + calculate the number of neighbours
                // + + +
                sum = int(strip[mod((y-1), height)][mod((x-1), width)]) + int(strip[mod((y-1), height)][mod((x), width)]) + int(strip[mod((y-1), height)][mod((x+1), width)]) +
                      int(strip[mod((y), height)][mod((x-1), width)])   + int(strip[(y)%height][(x+1)%width]) +
                      int(strip[mod((y+1), height)][mod((x-1), width)]) + int(strip[(y+1)%height][(x)%width]) + int(strip[(y+1)%height][(x+1)%width])
                // division by 255 because an alive cell is stored as 255 in the image file
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
    }

    // send the cell data back to the distributor after all the turns have been
    // completed
    for y := 1; y < height-1; y++ {
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

    // create all the channels for the halo exchange
    sendChans := make([]chan<- byte, 2*p.threads)
    recChans := make([]<-chan byte, 2*p.threads)

    // these channels are for sending and receiving the bytes to and from the workers
    disChans := make([]chan byte, p.threads)

    // these channels are for telling the workers to perform their next turn
    tickChans := make([]chan bool, p.threads)

    // these channels are for telling the workers to output their current data
    outSliceChans := make([]chan bool, p.threads)

    for i := 0; i < 2*p.threads; i++ {
        c := make(chan byte, 2*p.imageWidth) //double so pause works, allows 2 halo's to be sent, so doesn't dead lock
        sendChans[i] = c
        recChans[i] = c
    }

    keyChan := make(chan string)
    go getKeyboardCommand(keyChan)
    t := time.NewTicker(time.Second * 2)

    //sending data to the workers
    for i := 0; i < p.threads; i++ {
        disChans[i] = make(chan byte)
        tickChans[i] = make(chan bool)
        outSliceChans[i] = make(chan bool)

        lowerBound := int(math.Round(float64(p.imageHeight*i) / float64(p.threads)))
        upperBound := int(math.Round((float64(p.imageHeight*(i+1)) / float64(p.threads))))

        // assign the correct channels to the worker
        var wc workerChans
        wc.disChan = disChans[i]
        wc.aboveSend = sendChans[i*2]
        wc.aboveRec = recChans[mod((i*2)-1, 2*p.threads)]
        wc.belowSend = sendChans[(i*2)+1]
        wc.belowRec = recChans[mod((i+1)*2, 2*p.threads)]
        wc.outSlice = outSliceChans[i]

        go worker(i, int(upperBound-lowerBound)+2, p.imageWidth, p.turns, wc, tickChans[i])

        for y := lowerBound - 1; y < upperBound+1; y++ {
            for x := 0; x < p.imageWidth; x++ {
                disChans[i] <- world[mod(y, p.imageHeight)][x]
            }
        }
    }

    paused := false
    quit := false
    for n := 0; n < p.turns && !quit; n++ {
        select {
        // this case is run every 2 seconds to print out the number of alive cells
        case <-t.C:
            
            requestDataFromWorkers(outSliceChans)

            // sum up the number of alive cells in the world
            sum := 0
            for i := 0; i < p.threads; i++ {

                lowerBound := math.Round(float64(p.imageHeight*i) / float64(p.threads))
                upperBound := math.Round((float64(p.imageHeight*(i+1)) / float64(p.threads)))

                for y := lowerBound; y < upperBound; y++ {
                    for x := 0; x < p.imageWidth; x++ {
                        sum += int(<-disChans[i])
                    }
                }
            }
            sum /= 255

            fmt.Printf("The number of alive cells are - %d\n", sum)

        // this case is run when the user presses a key
        case c := <-keyChan:
            if c == "p" {
                // if we are paused unpause
                // this code should never run
                if paused {
                    paused = false
                    fmt.Println("Continuing")

                // called when we are not paused and 'p' is pressed
                } else {
                    fmt.Printf("Paused %d\n", n)
                    paused = true

                    // while we are paused and 'q' has not been pressed
                    for paused && !quit {
                        // this select is here so that whilst the simulation is paused

                        // the keys can still be pressed and used
                        // this does not busy wait because we wait on keyChan

                        // the select also does not have a default case, this is because
                        // the user must press a key to exit the paused state.
                        select {
                        case k := <-keyChan:
                            if k == "p" {
                                paused = false
                                fmt.Println("Continuing")
                            } else if k == "s" {

                                requestDataFromWorkers(outSliceChans)
                                sPressed(p, d, world, disChans, n)

                            } else if k == "q" {
                                fmt.Println("q")
                                quit = true

                                // we only request the data and the data is written after the turns loop
                                // has exited
                                requestDataFromWorkers(outSliceChans)
                            }
                        }
                    }
                }
            } else if c == "s" {

                requestDataFromWorkers(outSliceChans)
                sPressed(p, d, world, disChans, n)

            // if q is pressed tell the workers to send the world to the distributor
            // also terminate the program
            } else if c == "q" {
                fmt.Println("q outer")
                quit = true

                // we only request the data and the data is written after the turns loop
                // has exited
                requestDataFromWorkers(outSliceChans)
            }
        // the default case exists to ensure that the execution can still continue
        // even if the user does not press a key
        default:
        }

        for j := 0; j < p.threads && !quit; j++ {
            tickChans[j] <- true
        }
    }

    // receiving data from the workers and reconstructing
    for i := 0; i < p.threads; i++ {

        lowerBound := math.Round(float64(p.imageHeight*i) / float64(p.threads))
        upperBound := math.Round((float64(p.imageHeight*(i+1)) / float64(p.threads)))

        for y := lowerBound; y < upperBound; y++ {
            for x := 0; x < p.imageWidth; x++ {
                world[int(y)][x] = <-disChans[i]
            }
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

func sPressed(p golParams, d distributorChans, world [][]byte, disChans []chan byte, n int) {

    // recombine the board with the data from the workers
    for i := 0; i < p.threads; i++ {

        lowerBound := math.Round(float64(p.imageHeight*i) / float64(p.threads))
        upperBound := math.Round((float64(p.imageHeight*(i+1)) / float64(p.threads)))

        for y := lowerBound; y < upperBound; y++ {
            for x := 0; x < p.imageWidth; x++ {
                world[int(y)][x] = <-disChans[i]
            }
        }
    }

    d.io.command <- ioOutput
    d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x") + "t" + strconv.Itoa(n)

    // The io goroutine sends the requested image byte by byte, in rows.
    for y := 0; y < p.imageHeight; y++ {
        for x := 0; x < p.imageWidth; x++ {
            d.io.outputVal <- world[y][x]
        }
    }
}

func requestDataFromWorkers(outSliceChans []chan bool) {
    for _, outSliceChan := range outSliceChans {
        outSliceChan <- true
    }
}
