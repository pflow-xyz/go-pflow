/**
 * Karate Game Client
 *
 * A JavaScript client module for connecting to the karate fighting game server.
 * Styled after pflow-xyz for consistency with the go-pflow ecosystem.
 *
 * @example
 * const client = new KarateClient('ws://localhost:8080/ws');
 *
 * client.on('match_found', (data) => {
 *   console.log('Match started!', data);
 * });
 *
 * client.on('game_state', (state) => {
 *   renderGame(state);
 * });
 *
 * client.connect();
 * client.joinGame('player123', 'ai'); // or 'pvp' for matchmaking
 */

class KarateClient {
    /**
     * Create a new KarateClient
     * @param {string} wsUrl - WebSocket server URL (e.g., 'ws://localhost:8080/ws')
     */
    constructor(wsUrl) {
        this.wsUrl = wsUrl;
        this.ws = null;
        this.connected = false;
        this.playerId = null;
        this.playerNum = null;
        this.sessionId = null;
        this.isVsAI = false;
        this.gameState = null;
        this.availableActions = [];

        // Event handlers
        this.handlers = {
            'connect': [],
            'disconnect': [],
            'error': [],
            'match_found': [],
            'game_state': [],
            'action_result': [],
            'game_over': [],
            'chat': [],
        };

        // Reconnection settings
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 1000;

        // Ping interval
        this.pingInterval = null;
    }

    /**
     * Register an event handler
     * @param {string} event - Event name
     * @param {Function} handler - Handler function
     * @returns {KarateClient} - Returns this for chaining
     */
    on(event, handler) {
        if (this.handlers[event]) {
            this.handlers[event].push(handler);
        }
        return this;
    }

    /**
     * Remove an event handler
     * @param {string} event - Event name
     * @param {Function} handler - Handler function to remove
     * @returns {KarateClient} - Returns this for chaining
     */
    off(event, handler) {
        if (this.handlers[event]) {
            this.handlers[event] = this.handlers[event].filter(h => h !== handler);
        }
        return this;
    }

    /**
     * Emit an event to all registered handlers
     * @param {string} event - Event name
     * @param {*} data - Event data
     */
    emit(event, data) {
        if (this.handlers[event]) {
            this.handlers[event].forEach(handler => {
                try {
                    handler(data);
                } catch (err) {
                    console.error(`Error in ${event} handler:`, err);
                }
            });
        }
    }

    /**
     * Connect to the game server
     * @returns {Promise} - Resolves when connected
     */
    connect() {
        return new Promise((resolve, reject) => {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                resolve();
                return;
            }

            this.ws = new WebSocket(this.wsUrl);

            this.ws.onopen = () => {
                this.connected = true;
                this.reconnectAttempts = 0;
                this.startPing();
                this.emit('connect', {});
                resolve();
            };

            this.ws.onclose = (event) => {
                this.connected = false;
                this.stopPing();
                this.emit('disconnect', { code: event.code, reason: event.reason });

                // Attempt reconnection
                if (this.reconnectAttempts < this.maxReconnectAttempts) {
                    this.reconnectAttempts++;
                    setTimeout(() => this.connect(), this.reconnectDelay * this.reconnectAttempts);
                }
            };

            this.ws.onerror = (error) => {
                this.emit('error', { message: 'WebSocket error', error });
                reject(error);
            };

            this.ws.onmessage = (event) => {
                this.handleMessage(event.data);
            };
        });
    }

    /**
     * Disconnect from the server
     */
    disconnect() {
        this.maxReconnectAttempts = 0; // Prevent auto-reconnect
        if (this.ws) {
            this.send('leave', {});
            this.ws.close();
            this.ws = null;
        }
        this.stopPing();
    }

    /**
     * Send a message to the server
     * @param {string} type - Message type
     * @param {Object} payload - Message payload
     */
    send(type, payload) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.error('WebSocket not connected');
            return;
        }

        const message = {
            type: type,
            payload: payload,
            timestamp: Date.now()
        };

        this.ws.send(JSON.stringify(message));
    }

    /**
     * Handle incoming messages
     * @param {string} data - Raw message data
     */
    handleMessage(data) {
        try {
            const message = JSON.parse(data);
            const payload = message.payload;

            switch (message.type) {
                case 'match_found':
                    this.sessionId = payload.session_id;
                    this.playerNum = payload.player;
                    this.isVsAI = payload.is_vs_ai;
                    this.emit('match_found', payload);
                    break;

                case 'game_state':
                    this.gameState = payload.state;
                    this.availableActions = payload.available_actions || [];
                    this.emit('game_state', {
                        state: this.gameState,
                        availableActions: this.availableActions,
                        yourPlayer: payload.your_player
                    });
                    break;

                case 'action_result':
                    this.emit('action_result', payload);
                    break;

                case 'game_over':
                    this.gameState = payload.state;
                    this.emit('game_over', payload);
                    break;

                case 'error':
                    this.emit('error', payload);
                    break;

                case 'pong':
                    // Ping acknowledged
                    break;

                default:
                    console.log('Unknown message type:', message.type);
            }
        } catch (err) {
            console.error('Error parsing message:', err);
        }
    }

    /**
     * Join a game
     * @param {string} playerId - Player identifier
     * @param {string} mode - Game mode: 'ai' for single player, 'pvp' for matchmaking
     */
    joinGame(playerId, mode = 'ai') {
        this.playerId = playerId;
        this.send('join', {
            player_id: playerId,
            mode: mode
        });
    }

    /**
     * Join the matchmaking queue for PvP
     */
    joinMatchmaking() {
        this.send('matchmaking', {});
    }

    /**
     * Submit an action
     * @param {string} action - Action to perform: 'punch', 'kick', 'special', 'block', 'move_left', 'move_right', 'recover'
     */
    submitAction(action) {
        if (!this.availableActions.includes(action)) {
            console.warn(`Action '${action}' not available. Available: ${this.availableActions.join(', ')}`);
        }
        this.send('action', { action: action });
    }

    /**
     * Signal that the player is ready
     */
    ready() {
        this.send('ready', {});
    }

    /**
     * Leave the current game
     */
    leaveGame() {
        this.send('leave', {});
        this.sessionId = null;
        this.playerNum = null;
        this.gameState = null;
        this.availableActions = [];
    }

    /**
     * Start ping interval to keep connection alive
     */
    startPing() {
        this.pingInterval = setInterval(() => {
            this.send('ping', {});
        }, 25000);
    }

    /**
     * Stop ping interval
     */
    stopPing() {
        if (this.pingInterval) {
            clearInterval(this.pingInterval);
            this.pingInterval = null;
        }
    }

    /**
     * Get current game state
     * @returns {Object|null} - Current game state or null if not in game
     */
    getState() {
        return this.gameState;
    }

    /**
     * Get available actions for current state
     * @returns {string[]} - List of available action names
     */
    getAvailableActions() {
        return this.availableActions;
    }

    /**
     * Check if an action is currently available
     * @param {string} action - Action name
     * @returns {boolean}
     */
    canDoAction(action) {
        return this.availableActions.includes(action);
    }

    /**
     * Check if in an active game
     * @returns {boolean}
     */
    isInGame() {
        return this.sessionId !== null && this.gameState !== null && !this.gameState.game_over;
    }

    /**
     * Check if game is over
     * @returns {boolean}
     */
    isGameOver() {
        return this.gameState?.game_over || false;
    }

    /**
     * Get the winner (if game is over)
     * @returns {number|null} - Player number (1 or 2) or null if no winner yet
     */
    getWinner() {
        return this.gameState?.winner || null;
    }

    /**
     * Check if this client won
     * @returns {boolean}
     */
    didWin() {
        return this.getWinner() === this.playerNum;
    }
}

// Action constants
KarateClient.Actions = {
    PUNCH: 'punch',
    KICK: 'kick',
    SPECIAL: 'special',
    BLOCK: 'block',
    MOVE_LEFT: 'move_left',
    MOVE_RIGHT: 'move_right',
    RECOVER: 'recover'
};

// Game constants
KarateClient.Constants = {
    MAX_HEALTH: 100,
    MAX_STAMINA: 50,
    NUM_POSITIONS: 5,
    PUNCH_DAMAGE: 10,
    KICK_DAMAGE: 15,
    SPECIAL_DAMAGE: 25,
    BLOCK_REDUCTION: 0.5,
    PUNCH_STAMINA: 5,
    KICK_STAMINA: 8,
    SPECIAL_STAMINA: 15,
    BLOCK_STAMINA: 3,
    MOVE_STAMINA: 2,
    STAMINA_RECOVERY: 2
};

// Export for different module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = KarateClient;
} else if (typeof window !== 'undefined') {
    window.KarateClient = KarateClient;
}
