package stubs

import "uk.ac.bris.cs/gameoflife/util"

var EvolveWorld = "GameofLife.EvolveWorld"
var GetAliveCells = "GameofLife.GetAliveCells"
var DealWithKeyPresses = "GameofLife.DealWithKeyPresses"

type Response struct {
	World       [][]byte
	AliveCells  []util.Cell
	AliveCells2 int
	Turn        int
}

type Request struct {
	World       [][]byte
	Turn        int
	ImageHeight int
	ImageWidth  int
	Threads     int
}

type DealKeyPresses struct {
	Key rune
}

type KeyPressResponse struct{
	World[][] byte
	CurrentTurn int
}
