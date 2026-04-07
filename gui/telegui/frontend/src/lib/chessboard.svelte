<script lang="ts">
  import kw from "../assets/images/kw.png";
  import qw from "../assets/images/qw.png";
  import rw from "../assets/images/rw.png";
  import bw from "../assets/images/bw.png";
  import nw from "../assets/images/nw.png";
  import pw from "../assets/images/pw.png";
  import kb from "../assets/images/kb.png";
  import qb from "../assets/images/qb.png";
  import rb from "../assets/images/rb.png";
  import bb from "../assets/images/bb.png";
  import nb from "../assets/images/nb.png";
  import pb from "../assets/images/pb.png";
  import { EventsEmit, EventsOn } from "../../wailsjs/runtime/runtime.js";

  let validMoves = ["e4e5", "e4d5", "d4d6"];
  var imageMappings = {
    1: kw,
    2: qw,
    3: rw,
    4: bw,
    5: nw,
    6: pw,
    7: kb,
    8: qb,
    9: rb,
    10: bb,
    11: nb,
    12: pb,
  };
  var squareMap = [];
  let squares = Array.from({ length: 64 }, (_, i) => ({
    color: (Math.floor(i / 8) + (i % 8)) % 2 === 0 ? "light" : "dark",
    piece: squareMap[i],
    squareId: `sq${i}`,
    squareIndex: i,
  }));
  let OriginSquare = "";
  function convertIndexToSquare(index: number): string {
    index = 63-index;
    const files = ["h", "g", "f", "e", "d", "c", "b", "a"];
    const ranks = ["1", "2", "3", "4", "5", "6", "7", "8"];

    const file = files[index % 8];
    const rank = ranks[Math.floor(index / 8)];

    return `${file}${rank}`;
  }
  function convertSquareToIndex(square: string): number {
    const files = ["h", "g", "f", "e", "d", "c", "b", "a"];
    const ranks = ["1", "2", "3", "4", "5", "6", "7", "8"];

    const file = square[0];
    const rank = square[1];

    const fileIndex = files.indexOf(file);
    const rankIndex = ranks.indexOf(rank);

    if (fileIndex === -1 || rankIndex === -1) {
      throw new Error("Invalid square notation");
    }

    return 63 - (rankIndex * 8 + fileIndex);
  }
  
  function handleSquareClick(index: number) {
    let allPointers = document.getElementsByClassName("pointer");
    for (let j = 0; j < allPointers.length; j++) {
      let curr = allPointers[j] as HTMLButtonElement;
      curr.classList.add("invis");
      curr.disabled = true;
    }
    let square = convertIndexToSquare(index);
    OriginSquare = square;
    console.log(`Square clicked: ${square}`);
    for (let i = 0; i < validMoves.length; i++) {
      if (validMoves[i].startsWith(square)) {
        console.log(`Valid move: ${validMoves[i]}`);
        let move = validMoves[i].substring(2, 4);
        let idx = convertSquareToIndex(move);
        console.log(`Move to index: ${idx}`);

        let pointer = document.getElementById(`pointer-${idx}`) as HTMLButtonElement;
        if (pointer) {
          pointer.classList.remove("invis");
          pointer.disabled = false;
        }
      }
    }
  }
  function reRenderBoard(newSquareMap: number[]) {
    squareMap = newSquareMap;
    squares = Array.from({ length: 64 }, (_, i) => ({
      color: (Math.floor(i / 8) + (i % 8)) % 2 === 0 ? "light" : "dark",
      piece: squareMap[i],
      squareId: `sq${i}`,
      squareIndex: i,
    }));
  }

  function resolveMoveToPlay(originSquare: string, destinationSquare: string): string {
    const baseMove = originSquare + destinationSquare;
    const candidates = validMoves.filter((move) => move.startsWith(baseMove));

    if (candidates.length === 0) {
      return baseMove;
    }

    // If this is a promotion move, default to queen promotion.
    const queenPromotion = candidates.find((move) => move === `${baseMove}q`);
    if (queenPromotion) {
      return queenPromotion;
    }

    return candidates[0];
  }

  function handlePointerClick(index: number) {
    let square = convertIndexToSquare(index);
    let moveToPlay = resolveMoveToPlay(OriginSquare, square);
    EventsEmit("movePlayed", moveToPlay);
    console.log(`Move played: ${moveToPlay}`);
    let allPointers = document.getElementsByClassName("pointer");
    for (let j = 0; j < allPointers.length; j++) {
      let curr = allPointers[j] as HTMLButtonElement;
      curr.classList.add("invis");
      curr.disabled = true;
    }
  }
  
  EventsOn("squareMap:updated", (payload: string) => {
    console.log("Received new square map:", payload);
    const jsonPayload = JSON.parse(payload);
    reRenderBoard(jsonPayload.squareMap);
    validMoves = jsonPayload.vmUci;
  });
</script>

<div class="boardContainer">
  <div class="board">
    {#each squares as square}
      <div class="square {square.color}" id="square-{square.squareId}">
        <button class="pointer invis" id="pointer-{square.squareIndex}" disabled on:click={() => handlePointerClick(square.squareIndex)}></button>
        {#if square.piece}
          <div class="piece">
            <button
              class="piece-button"
              on:click={() => handleSquareClick(square.squareIndex)}
            >
              <img
                src={imageMappings[square.piece]}
                alt="Piece"
                class="piece-image"
              />
            </button>
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .invis {
    display: none;
  }
  .pointer {
    outline: none;
    border: none;
    background-color: black;
    position: absolute;

    height: 70%;
    width: 70%;
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: radial-gradient(
      rgba(0, 216, 169, 0.644),
      rgba(173, 177, 230, 0.438)
    );
    border-radius: 200px;
  }
  .piece-button {
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
  }
  .piece-image {
    width: 60%;
    aspect-ratio: 1/1;
  }
  .square {
    position: relative;
    aspect-ratio: 1/1;
    width: auto;
    display: flex;
    justify-content: center;
    align-items: center;
  }
  .piece {
    display: flex;
    justify-content: center;
    align-items: center;
    width: 100%;
    height: 100%;
  }
  .light {
    background-color: rgb(231, 236, 235);
  }
  .dark {
    background-color: rgb(71, 78, 80);
  }
  .board {
    display: grid;
    grid-template-columns: repeat(8, 1fr);
    grid-template-rows: repeat(8, 1fr);
    width: 100%;
    aspect-ratio: 1/1;
    border-radius: 200px;
  }
  .boardContainer {
    display: flex;
    justify-content: center;
    align-items: center;
    width: 50%;
    padding: 50px;
  }
</style>
