<script lang="ts">
    import {EventsOn} from "../../wailsjs/runtime/runtime.js";
    let gameOverReason = "";
    let gameOver = false
    EventsOn("gameOver", (payload: string) => {
        console.log("Game over event received with reason:", payload);
        gameOverReason = payload;
        gameOver = true;
    });
</script>
{#if gameOver}
    <div class="ovr">
        <div class="cont">Game Over</div>
        <div class="reason">{gameOverReason}</div>
    </div>
{/if}
<style>
    .ovr{
        position: absolute;
        height: 100vh;
        width: 100vw;
        display: flex;
        flex-direction: column;
        justify-content: center;
        align-items: center;
        background-color: rgba(0, 0, 0, 0.5);
        z-index: 10;
        font-family: sans-serif;
    }
    .cont{
        font-size: 50px;
        color:white;
    }
    .reason{
        font-size: 20px;
        color:white;
        margin-top: 10px;
    }
</style>
