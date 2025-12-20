const puppeteer = require('puppeteer');

// Helper function for delays
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function testCombatUI() {
    console.log('Launching browser...');
    const browser = await puppeteer.launch({
        headless: 'new',
        args: ['--no-sandbox', '--disable-setuid-sandbox']
    });

    const page = await browser.newPage();

    // Collect console logs
    page.on('console', msg => {
        console.log('BROWSER:', msg.type(), msg.text());
    });

    // Collect errors
    page.on('pageerror', err => {
        console.log('PAGE ERROR:', err.message);
    });

    console.log('Navigating to game...');
    // Use the seed that triggers combat scenarios
    await page.goto('http://localhost:8082/?seed=1764689418250597600', { waitUntil: 'networkidle0' });

    // Wait for game to load
    await page.waitForSelector('#ascii-view', { timeout: 5000 });
    console.log('Game loaded!');

    // Wait for WebSocket to connect and receive game state
    console.log('Waiting for game state...');

    // Wait up to 5 seconds for gameState to be populated
    // Note: gameState is let-scoped, but we exposed window.getGameState() for testing
    let gameStateReady = false;
    for (let i = 0; i < 50; i++) {
        const hasState = await page.evaluate(() => {
            const gs = window.getGameState ? window.getGameState() : null;
            return !!gs;
        });
        if (hasState) {
            gameStateReady = true;
            break;
        }
        await delay(100);
    }
    console.log('Game state ready:', gameStateReady);

    // Check WebSocket status via exposed getter
    const wsStatus = await page.evaluate(() => {
        const gs = window.getGameState ? window.getGameState() : null;
        return {
            hasGetter: typeof window.getGameState === 'function',
            gameStateReceived: !!gs,
            gameStateHasPlayer: gs && !!gs.player
        };
    });
    console.log('WebSocket status (via getGameState):', wsStatus);

    // Get initial game state and find enemy positions
    const initialState = await page.evaluate(() => {
        const gs = window.getGameState ? window.getGameState() : null;
        return {
            hasGameState: !!gs,
            playerPos: gs ? {x: gs.player.x, y: gs.player.y} : null,
            enemies: gs ? gs.enemies.map(e => ({name: e.name, x: e.x, y: e.y, state: e.state, alert_dist: e.alert_dist || 5})) : [],
            combat: gs ? gs.combat : null
        };
    });
    console.log('Initial state:', JSON.stringify(initialState, null, 2));

    // Find nearest alive enemy
    let nearestEnemy = null;
    let nearestDist = Infinity;
    if (initialState.enemies.length > 0) {
        for (const e of initialState.enemies) {
            if (e.state === 'dead') continue;
            const dx = initialState.playerPos.x - e.x;
            const dy = initialState.playerPos.y - e.y;
            const dist = Math.sqrt(dx*dx + dy*dy);
            if (dist < nearestDist) {
                nearestDist = dist;
                nearestEnemy = e;
            }
        }
    }
    console.log('Nearest enemy:', nearestEnemy, 'distance:', nearestDist.toFixed(2));

    // Move toward the enemy
    if (nearestEnemy) {
        console.log('\nMoving toward enemy...');
        const maxMoves = 50;
        for (let i = 0; i < maxMoves; i++) {
            const currentState = await page.evaluate(() => {
                const gs = window.getGameState ? window.getGameState() : null;
                if (!gs) return null;
                return {
                    playerPos: {x: gs.player.x, y: gs.player.y},
                    combat: gs.combat
                };
            });

            if (!currentState) break;
            if (currentState.combat && currentState.combat.active) {
                console.log('Combat started at move', i);
                break;
            }

            const px = currentState.playerPos.x;
            const py = currentState.playerPos.y;
            const dx = nearestEnemy.x - px;
            const dy = nearestEnemy.y - py;
            const dist = Math.sqrt(dx*dx + dy*dy);

            if (dist <= 2) {
                console.log('Close to enemy at distance', dist.toFixed(2));
                break;
            }

            // Move in direction of enemy
            if (Math.abs(dx) > Math.abs(dy)) {
                await page.keyboard.press(dx > 0 ? 'KeyD' : 'KeyA');
            } else {
                await page.keyboard.press(dy > 0 ? 'KeyS' : 'KeyW');
            }
            await delay(80);
        }
    }

    // Check state after moving
    const afterMove = await page.evaluate(() => {
        const gs = window.getGameState ? window.getGameState() : null;
        return {
            playerPos: gs ? {x: gs.player.x, y: gs.player.y} : null,
            nearbyEnemies: gs ? gs.enemies.filter(e => {
                const dx = gs.player.x - e.x;
                const dy = gs.player.y - e.y;
                return Math.sqrt(dx*dx + dy*dy) <= 6;
            }).map(e => ({name: e.name, x: e.x, y: e.y, state: e.state})) : [],
            combat: gs ? gs.combat : null
        };
    });
    console.log('After moving:', JSON.stringify(afterMove, null, 2));

    // Try to initiate combat with Tab
    console.log('\nPressing Tab to initiate combat...');
    await page.keyboard.press('Tab');
    await delay(500);

    // Check combat state
    const combatState = await page.evaluate(() => {
        const gs = window.getGameState ? window.getGameState() : null;
        return {
            combat: gs ? gs.combat : null,
            combatPanelVisible: document.getElementById('combat-panel').style.display,
            combatPanelHTML: document.getElementById('combat-panel').innerHTML.substring(0, 500)
        };
    });
    console.log('\nCombat state after Tab:', JSON.stringify(combatState, null, 2));

    // Check if combat panel is visible
    const isPanelVisible = await page.evaluate(() => {
        const panel = document.getElementById('combat-panel');
        const style = window.getComputedStyle(panel);
        return {
            display: style.display,
            visibility: style.visibility,
            opacity: style.opacity
        };
    });
    console.log('Combat panel visibility:', isPanelVisible);

    // If combat is active, try an attack
    if (combatState.combat && combatState.combat.active) {
        console.log('\nCombat is active! Trying attack (A key)...');
        await page.keyboard.press('KeyA');
        await delay(500);

        const afterAttack = await page.evaluate(() => {
            const gs = window.getGameState ? window.getGameState() : null;
            return {
                combat: gs ? gs.combat : null,
                messages: gs ? gs.message_log.slice(-5) : []
            };
        });
        console.log('After attack:', JSON.stringify(afterAttack, null, 2));
    }

    // Take a screenshot
    await page.screenshot({ path: '/tmp/combat_test.png', fullPage: true });
    console.log('\nScreenshot saved to /tmp/combat_test.png');

    await browser.close();
    console.log('\nTest complete!');
}

testCombatUI().catch(err => {
    console.error('Test failed:', err);
    process.exit(1);
});
