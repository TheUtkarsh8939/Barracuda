<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { EventsOn } from "../../wailsjs/runtime/runtime.js";

  type BoardStatePayload = {
    movesPlayed?: string[];
  };

  let moveList: string[] = [];
  let isTurnWhite = true;
  let timeRemainingWhiteSeconds = 600;
  let timeRemainingBlackSeconds = 120;
  let WhiteTimeFormatted = "10:00";
  let BlackTimeFormatted = "2:00";
  let isFrozenTimer = false;
  EventsOn("gameOver", (payload: string) => {
    isFrozenTimer = true;
  });
  const formatTime = (seconds: number): string => {
    const safeSeconds = Math.max(0, seconds);
    const mins = Math.floor(safeSeconds / 60);
    const secs = safeSeconds % 60;
    return `${mins}:${secs < 10 ? "0" : ""}${secs}`;
  };

  const refreshFormattedTimes = (): void => {
    WhiteTimeFormatted = formatTime(timeRemainingWhiteSeconds);
    BlackTimeFormatted = formatTime(timeRemainingBlackSeconds);
  };

  let cleanupEventListener: (() => void) | undefined;
  let timerId: ReturnType<typeof setInterval> | undefined;

  onMount(() => {
    cleanupEventListener = EventsOn("squareMap:updated", (payload: string) => {
      let parsed: BoardStatePayload;
      try {
        parsed = JSON.parse(payload) as BoardStatePayload;
      } catch {
        return;
      }

      moveList = Array.isArray(parsed.movesPlayed) ? parsed.movesPlayed : [];
      // White to move on even plies (0, 2, 4...), black on odd plies.
      isTurnWhite = moveList.length % 2 === 0;
    });

    timerId = setInterval(() => {
      if (isFrozenTimer) {
        return;
      }
      if (isTurnWhite) {
        timeRemainingWhiteSeconds = Math.max(0, timeRemainingWhiteSeconds - 1);
      } else {
        timeRemainingBlackSeconds = Math.max(0, timeRemainingBlackSeconds - 1);
      }
      refreshFormattedTimes();
    }, 1000);

    refreshFormattedTimes();
  });

  onDestroy(() => {
    if (timerId) {
      clearInterval(timerId);
    }
    cleanupEventListener?.();
  });
</script>

<div class="cont">
  <div class="innerCont">
    <div class="timer">
      <div class="player" id="player1">
        <div class="timeRemaining">{WhiteTimeFormatted}</div>
        <div class="playerName">
          <div class="white"></div>
          You
        </div>
      </div>
      <div class="player" id="player2">
        <div class="timeRemaining">{BlackTimeFormatted}</div>
        <div class="playerName">
          <span style="font-size: 10px;">Barracuda</span>
          <div class="black"></div>
        </div>
      </div>
    </div>
    <div class="moves">
      <div class="opening">Van Geet Opening</div>
      <div class="allMoves">
        {#each moveList as move}
          <div class="move">{move}</div>
        {/each}
        <!-- <div class="move">1. e4 e5</div>
        <div class="move">2. Nf3 Nc6</div> -->
      </div>
    </div>
  </div>
</div>

<style>
  .opening {
    height: 40px;
    padding-top: 20px;
    padding-left: 20px;
    width: 90%;
    color: rgb(189, 189, 189);
    font-size: large;
    font-family: sans-serif;
    border-bottom: 1px solid rgb(71, 78, 80);
  }
  .allMoves {
    height: calc(100% - 40px);
    width: 100%;
    padding-top: 20px;
    display: flex;
    flex-direction: column;
    justify-content: flex-start;
    align-items: center;
  }
  .move {
    padding: 10px;
    width: 90%;
    color: rgb(189, 189, 189);
    font-size: medium;
    font-family: sans-serif;
    border-bottom: 1px solid rgb(71, 78, 80);
  }
  .moves {
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    border: 1px solid rgb(71, 78, 80);
    height: calc(100% - 150px);
    margin-top: 30px;
    width: 90%;
    border-radius: 0px 0px 20px 20px;
    overflow-y: scroll;
  }
  .moves::-webkit-scrollbar {
    width: 10px;

    background: transparent; /* make scrollbar transparent */
  }
  .moves::-webkit-scrollbar-thumb {
    background: rgb(71, 78, 80); /* color of the scrollbar thumb */
    border-radius: 5px;
  }
  .white {
    width: 20px;
    height: 20px;
    background-color: white;
    border-radius: 20px;
    margin-right: 10px;
  }
  .black {
    width: 20px;
    height: 20px;
    background-color: black;
    border-radius: 20px;
    margin-left: 10px;
  }
  .timer {
    display: flex;
    justify-content: center;
    align-items: center;
    border: 1px solid rgb(71, 78, 80);
    height: 120px;
    width: 90%;
    border-radius: 20px 20px 0px 0px;
  }
  .player {
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    width: 50%;
    color: white;
    font-family: sans-serif;
    height: 100%;
  }
  #player1 {
    border-right: 1px solid rgb(71, 78, 80);
  }
  .playerName {
    font-size: 20px;
    font-weight: bold;
    font-family: sans-serif;
    height: 30%;
    width: 50%;
    text-align: center;
    border-top: 1px solid rgb(71, 78, 80);
    display: flex;
    justify-content: center;
    align-items: center;
  }
  .timeRemaining {
    font-size: 30px;
    font-weight: bold;
    font-family: sans-serif;
    height: 70%;
    width: 100%;
    text-align: center;
    display: flex;
    justify-content: center;
    align-items: center;
  }
  .cont {
    width: calc(50% - 130px);
    height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
  }
  .innerCont {
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    width: 100%;
    height: 85%;
    padding: 10px;
  }
</style>
