package main

import (
	"embed"

	"github.com/corentings/chess/v2"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func convertSquareMapToA8H1Index(src map[chess.Square]chess.Piece) map[int]chess.Piece {
	dst := make(map[int]chess.Piece, len(src))
	for sq, piece := range src {
		oldIndex := int(sq)
		file := oldIndex % 8
		rankFromBottom := oldIndex / 8
		newIndex := (7-rankFromBottom)*8 + file
		dst[newIndex] = piece
	}

	return dst
}

func main() {
	//Chess Loop
	game := chess.NewGame()
	squareMap := convertSquareMapToA8H1Index(game.Position().Board().SquareMap())
	vm := game.ValidMoves()
	vmUci := make([]string, len(vm))
	for i, move := range vm {
		vmUci[i] = move.String()
	}
	movesPlayed := game.Moves()
	positions := game.Positions()
	movesPlayedSan := make([]string, len(movesPlayed))
	algebraicNotation := chess.AlgebraicNotation{}
	for i, move := range movesPlayed {
		if i < len(positions) {
			movesPlayedSan[i] = algebraicNotation.Encode(positions[i], move)
			continue
		}
		movesPlayedSan[i] = move.String()
	}
	// Create an instance of the app structure
	app := NewApp(game, squareMap, vmUci, movesPlayedSan)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Barracuda GUI",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 19, B: 20, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

}
