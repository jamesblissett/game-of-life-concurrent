package main

import (
    "fmt"
    "github.com/nsf/termbox-go"
)

// getKeyboardCommand sends all keys pressed on the keyboard as runes (characters) on the key chan.
// getKeyboardCommand will NOT work if termbox isn't initialised (in startControlServer)
func getKeyboardCommand(keyChans []chan string, keyAvailable *bool) {
    for {
        event := termbox.PollEvent()
        if event.Type == termbox.EventKey {
            if event.Key != 0 {
              *keyAvailable = true
      				for _, keyChan := range keyChans {
      					keyChan <- string(rune(event.Key))
      				}
              *keyAvailable = false
            } else if event.Ch != 0 {
              *keyAvailable = true
      				for i, keyChan := range keyChans {
                fmt.Println(i)
      					keyChan <- string(event.Ch)
      				}
              *keyAvailable = false
            }
        }
    }
}

// startControlServer initialises termbox and prints basic information about the game configuration.
func startControlServer(p golParams) {
    e := termbox.Init()
    check(e)

    fmt.Println("Threads:", p.threads)
    fmt.Println("Width:", p.imageWidth)
    fmt.Println("Height:", p.imageHeight)
}

// stopControlServer closes termbox.
// If the program is terminated without closing termbox the terminal window may misbehave.
func StopControlServer() {
    termbox.Close()
}
