const puppeteer = require('puppeteer');

// Helper function for delays
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function testAICombat() {
    console.log('Launching browser...');
    const browser = await puppeteer.launch({
        headless: 'new',
        args: ['--no-sandbox', '--disable-setuid-sandbox']
    });

    const page = await browser.newPage();

    // Collect console logs
    page.on('console', msg => {
        if (msg.type() === 'error') {
            console.log('BROWSER ERROR:', msg.text());
        }
    });

    console.log('Navigating to game with seed...');
    await page.goto('http://localhost:8082/?seed=42', { waitUntil: 'networkidle0' });

    // Wait for game to load
    await page.waitForSelector('#ascii-view', { timeout: 5000 });
    console.log('Game loaded!');

    // Wait for game state
    for (let i = 0; i < 50; i++) {
        const hasState = await page.evaluate(() => {
            const gs = window.getGameState ? window.getGameState() : null;
            return !!gs;
        });
        if (hasState) break;
        await delay(100);
    }

    // Enable AI mode
    console.log('\nEnabling AI mode...');
    await page.click('#ai-toggle');
    await delay(500);

    // Wait and watch for combat to appear
    console.log('Running AI and watching for combat panel...');
    let combatSeen = false;
    let combatState = null;

    for (let i = 0; i < 100; i++) {
        const state = await page.evaluate(() => {
            const gs = window.getGameState ? window.getGameState() : null;
            if (!gs) return null;
            return {
                playerPos: { x: gs.player.x, y: gs.player.y },
                health: gs.player.health,
                combat: gs.combat,
                mode: gs.ai ? gs.ai.mode : 'unknown',
                lastAction: gs.ai ? gs.ai.last_action : ''
            };
        });

        if (!state) continue;

        // Check if combat is active
        if (state.combat && state.combat.active) {
            combatSeen = true;
            combatState = state.combat;
            console.log(`\n*** COMBAT DETECTED at tick ${i}! ***`);
            console.log('Player HP:', state.health);
            console.log('Combat round:', combatState.round_number);
            console.log('Player turn:', combatState.player_turn);
            console.log('AP:', combatState.current_ap + '/' + combatState.max_ap);
            console.log('Combatants:', combatState.combatants.length);

            // Check combat panel visibility
            const panelVisible = await page.evaluate(() => {
                const panel = document.getElementById('combat-panel');
                return panel ? panel.style.display : 'not found';
            });
            console.log('Combat panel display:', panelVisible);

            // Take screenshot of combat
            await page.screenshot({ path: '/tmp/ai_combat_test.png', fullPage: true });
            console.log('Screenshot saved to /tmp/ai_combat_test.png');
            break;
        }

        // Log progress every 10 ticks
        if (i % 10 === 0) {
            console.log(`Tick ${i}: pos=(${state.playerPos.x},${state.playerPos.y}) HP=${state.health} mode=${state.mode} action=${state.lastAction}`);
        }

        await delay(100);
    }

    if (!combatSeen) {
        console.log('\nNo combat detected in 100 ticks. Taking screenshot...');
        await page.screenshot({ path: '/tmp/ai_no_combat.png', fullPage: true });
        console.log('Screenshot saved to /tmp/ai_no_combat.png');
    }

    await browser.close();
    console.log('\nTest complete!');
    process.exit(combatSeen ? 0 : 1);
}

testAICombat().catch(err => {
    console.error('Test failed:', err);
    process.exit(1);
});
