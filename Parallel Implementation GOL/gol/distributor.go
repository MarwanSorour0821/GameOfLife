package gol

import (
	"fmt"
	"os"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
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

	// TODO: Create a 2D slice to store the world.

	nextWorld := make([][]byte, p.ImageHeight)
	for i := 0; i < p.ImageHeight; i++ {
		nextWorld[i] = make([]byte, p.ImageWidth)
	}

	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)

	turn := p.Turns

	getBytesFromInput(nextWorld, c.ioInput)

	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			if nextWorld[i][j] == 255 {
				c.events <- CellFlipped{0, util.Cell{X: j, Y: i}}
			}
		}
	}

	aliveCells := len(calculateAliveCells(p, nextWorld))
	ticker := time.NewTicker(2 * time.Second)
	var numTurns int
	paused := false

	if p.Threads == 1 {

		for t := 0; t < turn; t++ {
			select {
			case <-ticker.C:
				c.events <- AliveCellsCount{numTurns, aliveCells}
			case key := <-sdlKeyPresses:
				handleKeyPresses(key, c, &paused, numTurns, nextWorld, c.ioOutput, p)
			default:
			}

			if !paused {
				newWorld := calculateNextState(p, nextWorld, 0, p.ImageHeight, 0, p.ImageWidth, c, t)

				aliveCells = len(calculateAliveCells(p, newWorld))

				nextWorld = newWorld
				numTurns++
			}

		}

	} else {

		// TODO: Execute all turns of the Game of Life.
		for t := 0; t < turn; t++ {

			select {
			case <-ticker.C:
				c.events <- AliveCellsCount{numTurns, aliveCells}
			case key := <-sdlKeyPresses:
				handleKeyPresses(key, c, &paused, numTurns, nextWorld, c.ioOutput, p)
			default:
			}

			if !paused {

				var chans = make([]chan [][]byte, p.Threads)
				//oldWorld = nextWorld
				//remainingPixels := p.ImageHeight % p.Threads
				remainingPixels := getRemainder(p.ImageHeight, p.Threads)
				fromPixels := 0

				for i := 0; i < p.Threads; i++ {
					toPixels := fromPixels + p.ImageHeight/p.Threads
					if i < remainingPixels {
						toPixels++ // Distribute the remaining pixels among the first few threads
					}

					chans[i] = make(chan [][]byte)

					go worker(fromPixels, toPixels, 0, p.ImageWidth, p, nextWorld, chans[i], t, c)

					fromPixels = toPixels
				}

				var newWorld [][]uint8

				for _, elementChan := range chans {
					newWorld = append(newWorld, <-elementChan...)
				}

				copy(nextWorld, newWorld)

				// Notify that the turn has been complete
				// c.events <- CellFlipped{t,}
				c.events <- TurnComplete{t}
				aliveCells = len(calculateAliveCells(p, nextWorld))
				numTurns++
			}
		}
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, calculateAliveCells(p, nextWorld)}
	// Make sure that the IO has finished any output before exiting.

	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, p.Turns)

	sendToOutput(nextWorld, c.ioOutput)

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)

}

func calculateLiveNeighbours(world [][]byte, i int, j int) int {

	numOfLiveNeighbours := 0

	// up := getRemainder(getRemainder((i-1),len(world)))
	// //up := (getRemainder((i-1),len(world))) + (getRemainder(len(world), len(world)))
	// down := getRemainder((getRemainder((i+1),len(world))+ len(world)), len(world))
	// right := getRemainder(getRemainder((j+1),len(world[i])+len(world[i])),len(world[i]))
	// left := getRemainder((j-1), len(world[i])) + getRemainder(len(world[i]),len(world[i]))
	//positive modules
	// up := ((i-1)%len(world) + len(world)) % len(world)
	// down := ((i+1)%len(world) + len(world)) % len(world)
	// right := ((j+1)%len(world[i]) + len(world[i])) % len(world[i])
	// left := ((j-1)%len(world[i]) + len(world[i])) % len(world[i])
	up := getRemainder((getRemainder(i-1, len(world)) + len(world)), len(world))
	down := getRemainder(getRemainder(i+1, len(world))+len(world), len(world))
	right := getRemainder((getRemainder(j+1, len(world[i])) + len(world[i])), len(world[i]))
	left := getRemainder(getRemainder(j-1, len(world[i]))+len(world[i]), len(world[i]))

	neighbours := [8]byte{world[up][j], world[down][j], world[i][left], world[i][right], world[up][left], world[up][right], world[down][right], world[down][left]}

	for _, neighbour := range neighbours {
		if neighbour == 255 {
			numOfLiveNeighbours++
		}
	}

	return numOfLiveNeighbours
}

func calculateNextState(p Params, world [][]byte, startY, endY, startX, endX int, c distributorChannels, t int) [][]byte {

	nextWorld := make([][]byte, endY-startY)

	for i := startY; i < endY; i++ {
		nextWorld[i-startY] = make([]byte, endX)
		for j := startX; j < endX; j++ {
			liveNeighbours := calculateLiveNeighbours(world, i, j)

			// Apply the rules
			if world[i][j] == 255 {
				if liveNeighbours < 2 || liveNeighbours > 3 {
					nextWorld[i-startY][j] = 0 // Cell dies
					c.events <- CellFlipped{t, util.Cell{X: j, Y: i}}

				} else {
					nextWorld[i-startY][j] = 255 // Cell remains alive
				}
			} else {
				if liveNeighbours == 3 {
					nextWorld[i-startY][j] = 255 // Dead cell becomes alive
					c.events <- CellFlipped{t, util.Cell{X: j, Y: i}}
				} else {
					nextWorld[i-startY][j] = 0 // Cell remains dead
				}
			}
		}
	}
	return nextWorld
}

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	numRows := p.ImageHeight
	numColumns := p.ImageWidth
	cells := make([]util.Cell, 0)
	for row := 0; row < numRows; row++ {
		for col := 0; col < numColumns; col++ {
			cell1 := world[row][col]
			if cell1 == 255 {
				c := util.Cell{X: col, Y: row}
				cells = append(cells, c)
			}
		}
	}
	return cells
}

// initializeWorld initializes the world with input values.
func getBytesFromInput(world [][]byte, inputChannel <-chan byte) {
	for y := range world {
		for x := range world[y] {
			// Read values from the input channel and initialize the world.
			world[y][x] = <-inputChannel
		}
	}
}

func sendToOutput(world [][]byte, outputChannel chan<- byte) {

	for y := range world {
		for x := range world[y] {
			outputChannel <- world[y][x]
		}
	}
}

func worker(startY, endY, startX, endX int, p Params, world [][]byte, out chan<- [][]byte, t int, c distributorChannels) {
	out <- calculateNextState(p, world, startY, endY, startX, endX, c, t)
}

func handleKeyPresses(key rune, c distributorChannels, paused *bool, numTurns int, world [][]byte, outChan chan<- byte, p Params) {
	switch key {
	case 's':
		// Save the current state to a PGM file
		c.ioCommand <- ioOutput
		c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, p.Turns)
		sendToOutput(world, outChan)
		c.ioCommand <- ioCheckIdle
		<-c.ioIdle
		fmt.Println("Current state saved to PGM file.")
	case 'p':
		// Toggle pause and print the current turn
		*paused = !*paused
		if *paused {
			fmt.Printf("Game paused at turn %d.\n", numTurns)
		} else {
			fmt.Println("Resuming the game.")
		}
	case 'q':
		// Save the current state and exit the program
		c.ioCommand <- ioOutput
		c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, p.Turns)
		sendToOutput(world, outChan)
		c.ioCommand <- ioCheckIdle
		<-c.ioIdle
		fmt.Println("Current state saved to PGM file.")
		fmt.Println("Exiting the program.")
		os.Exit(0)
	default:
	}
}

func getRemainder(numerator, divisor int) (i int) {
	return (numerator - divisor*(numerator/divisor))

}
