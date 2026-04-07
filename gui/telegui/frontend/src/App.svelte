<script>
  import Sidebar from "./lib/sidebar.svelte";
  import Board from "./lib/chessboard.svelte"
  import GameInfo from "./lib/gameInfo.svelte";
  import GameOver from "./lib/gameOver.svelte";
</script>
<div class="app">
    <Sidebar />
    <Board />
    <GameInfo />
    <GameOver />
</div>
<style>
  .app {
    display: flex;
    flex-direction: row;
  }
</style>