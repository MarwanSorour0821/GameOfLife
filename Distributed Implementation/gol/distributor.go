package gol

import (
	"flag"
	"fmt"
	"net/rpc"

	//"os"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

// distributor divides the work between workers and interacts with other goroutines.

func distributor(p Params, c distributorChannels) {
	sdlKeyPresses := c.keyPresses

	server := "127.0.0.1:8030" //flag.String("server", "127.0.0.1:8030", "IP:port string to connect to as server")
	flag.Parse()
	client, err := rpc.Dial("tcp", server)

	if err != nil {
		// Handle error
		fmt.Println("Failed to connect to the server:", err)
		return
	}

	defer client.Close()

	// TODO: Create a 2D slice to store the world.
	nextWorld := make([][]byte, p.ImageHeight)
	for i := 0; i < p.ImageHeight; i++ {
		nextWorld[i] = make([]byte, p.ImageWidth)
	}

	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)

	getBytesFromInput(nextWorld, c.ioInput)

	turn := p.Turns

	// Prepare the RPC request
	request := stubs.Request{
		World:       nextWorld,
		Turn:        turn,
		ImageHeight: p.ImageHeight,
		ImageWidth:  p.ImageWidth,
		Threads:     p.Threads,
	}

	response := new(stubs.Response)

	// Make the RPC call to the EvolveWorld method
	//non blocking RPC call to allow for the next GetAliveCells RPC Call
	doneEvolve := client.Go("GameofLife.EvolveWorld", request, response, nil)
	if doneEvolve.Error != nil {
		// Handle error
		fmt.Println("RPC call failed:", err)
		return
	}

	//Make the RPC call to the EvolveWorld method
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	//paused := false

	dealWithKey := stubs.DealKeyPresses{
		Key: <-sdlKeyPresses,
	}

	//var res1 stubs.Response
	var req stubs.Request
	var dKey *stubs.KeyPressResponse
	// keys := make(chan rune)
	// keys <- dealWithKey.Key
	go func() {
		for {
			select {
			case <-doneEvolve.Done:
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				// fmt.Println("Sending alive to request")
				err1 := client.Call("GameofLife.GetAliveCells", request, response)
				if err1 != nil {
					// Handle error
					fmt.Println("RPC call number of cells failed:", err1)
					return
				}

				// Retrieve the alive cells count
				aliveCellsCount := len(response.AliveCells)
				// fmt.Printf("Alive Cells Count: %d\n", aliveCellsCount)

				// Send the event down the events channel
				c.events <- AliveCellsCount{response.Turn, aliveCellsCount}
			case key := <-sdlKeyPresses:
				fmt.Println("Key recieved")
				switch key {
				case 's':
					// Save the current state to a PGM file
					fmt.Println("Deal with kesssss")
					err1 := client.Call("GameofLife.DealWithKeyPresses", dealWithKey, &dKey)
					if err1 != nil {
						// Handle error
						fmt.Println("Deal with key not working: ", err1)
						return
					}
					c.ioCommand <- ioOutput
					c.ioFilename <- fmt.Sprintf("%dx%dx%d", req.ImageHeight, req.ImageWidth, dKey.CurrentTurn)
					sendToOutput(dKey.World, c.ioOutput)
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					fmt.Println("Current state saved to PGM file.")
				case 'p':
					// Toggle pause and print the current turn
					// *paused = !*paused
					// if *paused {
					// 	fmt.Printf("Game paused at turn %d.\n", numTurns)
					// } else {
					// 	fmt.Println("Resuming the game.")
					// }
				case 'q':
					//err1 := client.Call("GameofLife.DealWithKeyPresses", dealWithKey, res1)
					// Save the current state and exit the program
					// c.ioCommand <- ioOutput
					// c.ioFilename <- fmt.Sprintf("%dx%dx%d", req.ImageHeight, req.ImageWidth, res1.Turn)
					// sendToOutput(res1.World, c.ioOutput)
					// c.ioCommand <- ioCheckIdle
					// <-c.ioIdle
					// fmt.Println("Current state saved to PGM file.")
					// fmt.Println("Exiting the program.")
					// os.Exit(0)
				case 'k':
					return
				default:
					finishedWorld := response.World
					c.ioCommand <- ioOutput
					c.ioFilename <- fmt.Sprintf("%dx%dx%d", req.ImageHeight, req.ImageWidth, response.Turn)

					sendToOutput(finishedWorld, c.ioOutput)

					// TODO: Report the final state using FinalTurnCompleteEvent.
					c.events <- FinalTurnComplete{p.Turns, response.AliveCells}

					// Make sure that the Io has finished any output before exiting.
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
				}
			default:
				finishedWorld := response.World
				c.ioCommand <- ioOutput
				c.ioFilename <- fmt.Sprintf("%dx%dx%d", req.ImageHeight, req.ImageWidth, response.Turn)

				sendToOutput(finishedWorld, c.ioOutput)

				// TODO: Report the final state using FinalTurnCompleteEvent.
				c.events <- FinalTurnComplete{p.Turns, response.AliveCells}

				// Make sure that the Io has finished any output before exiting.
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle
			}
		}
	}()

	// finishedWorld := response.World
	// c.ioCommand <- ioOutput
	// c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, p.Turns)

	// sendToOutput(finishedWorld, c.ioOutput)

	// // TODO: Report the final state using FinalTurnCompleteEvent.
	// c.events <- FinalTurnComplete{p.Turns, response.AliveCells}

	// // Make sure that the Io has finished any output before exiting.
	// c.ioCommand <- ioCheckIdle
	// <-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func sendToOutput(world [][]byte, outputChannel chan<- byte) {
	for y := range world {
		for x := range world[y] {
			outputChannel <- world[y][x]
		}
	}
}

func getBytesFromInput(world [][]byte, inputChannel <-chan byte) {
	for y := range world {
		for x := range world[y] {
			// Read values from the input channel and initialize the world.
			world[y][x] = <-inputChannel
		}
	}
}

//func handleKeyPresses(key rune, c distributorChannels, paused *bool, numTurns int, world [][]byte, outChan chan<- byte, p Params) {
//
//	dealWithKey := stubs.DealKeyPresses{
//		Key: key,
//	}
//
//	var res1 stubs.Response
//	var req stubs.Request
//	switch key {
//	case 's':
//		// Save the current state to a PGM file
//		//err1 := client.Call("GameofLife.DealWithKeyPresses", dealWithKey, &res1)
//		c.ioCommand <- ioOutput
//		c.ioFilename <- fmt.Sprintf("%dx%dx%d", req.ImageHeight, req.ImageWidth, res1.Turn)
//		sendToOutput(res1.World, outChan)
//		c.ioCommand <- ioCheckIdle
//		<-c.ioIdle
//		fmt.Println("Current state saved to PGM file.")
//	case 'p':
//		// Toggle pause and print the current turn
//		*paused = !*paused
//		if *paused {
//			fmt.Printf("Game paused at turn %d.\n", numTurns)
//		} else {
//			fmt.Println("Resuming the game.")
//		}
//	case 'q':
//		//err1 := client.Call("GameofLife.DealWithKeyPresses", dealWithKey, res1)
//		// Save the current state and exit the program
//		c.ioCommand <- ioOutput
//		c.ioFilename <- fmt.Sprintf("%dx%dx%d", req.ImageHeight, req.ImageWidth, res1.Turn)
//		sendToOutput(res1.World, outChan)
//		c.ioCommand <- ioCheckIdle
//		<-c.ioIdle
//		fmt.Println("Current state saved to PGM file.")
//		fmt.Println("Exiting the program.")
//		os.Exit(0)
//	case 'k':
//
//	default:
//		// Handle other keys if needed
//	}
//}
