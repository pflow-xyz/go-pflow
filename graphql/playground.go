package graphql

import (
	"fmt"
	"net/http"
)

// PlaygroundHandler returns an HTTP handler that serves the GraphQL playground.
func PlaygroundHandler(playgroundPath, graphqlPath string) http.HandlerFunc {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>GraphQL Playground</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a2e; color: #eee; }
    .container { display: flex; height: 100vh; }
    .sidebar { width: 300px; background: #16213e; border-right: 1px solid #0f3460; display: flex; flex-direction: column; }
    .main { flex: 1; display: flex; flex-direction: column; }
    .header { padding: 16px; background: #0f3460; border-bottom: 1px solid #1a1a2e; }
    .header h1 { font-size: 18px; font-weight: 500; }
    .tabs { display: flex; background: #16213e; border-bottom: 1px solid #0f3460; }
    .tab { padding: 12px 20px; cursor: pointer; border-bottom: 2px solid transparent; }
    .tab.active { border-bottom-color: #e94560; color: #e94560; }
    .tab:hover { background: #1a1a2e; }
    .panel { flex: 1; overflow: auto; padding: 16px; }
    .editor-container { display: flex; flex: 1; }
    .editor { flex: 1; display: flex; flex-direction: column; border-right: 1px solid #0f3460; }
    .results { flex: 1; display: flex; flex-direction: column; }
    .editor-header, .results-header { padding: 8px 16px; background: #16213e; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; color: #888; }
    textarea { flex: 1; background: #1a1a2e; color: #eee; border: none; padding: 16px; font-family: 'Monaco', 'Menlo', monospace; font-size: 14px; resize: none; }
    textarea:focus { outline: none; }
    pre { flex: 1; background: #1a1a2e; color: #eee; padding: 16px; font-family: 'Monaco', 'Menlo', monospace; font-size: 14px; overflow: auto; margin: 0; white-space: pre-wrap; }
    .btn { background: #e94560; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; font-size: 14px; }
    .btn:hover { background: #ff6b6b; }
    .btn:disabled { background: #555; cursor: not-allowed; }
    .toolbar { padding: 12px 16px; background: #16213e; display: flex; gap: 12px; align-items: center; }
    .schema-type { margin-bottom: 16px; }
    .schema-type h3 { color: #e94560; margin-bottom: 8px; font-size: 14px; }
    .schema-field { padding: 4px 0; font-family: monospace; font-size: 13px; }
    .field-name { color: #4fc3f7; }
    .field-type { color: #81c784; }
    .field-args { color: #ffb74d; }
    .docs-section { margin-bottom: 24px; }
    .docs-section h2 { color: #e94560; margin-bottom: 12px; font-size: 16px; border-bottom: 1px solid #0f3460; padding-bottom: 8px; }
    .docs-section p { color: #aaa; line-height: 1.6; }
    code { background: #0f3460; padding: 2px 6px; border-radius: 3px; }
  </style>
</head>
<body>
  <div class="container">
    <div class="sidebar">
      <div class="header">
        <h1>GraphQL Playground</h1>
      </div>
      <div class="tabs">
        <div class="tab active" data-tab="schema">Schema</div>
        <div class="tab" data-tab="docs">Docs</div>
      </div>
      <div class="panel" id="schema-panel">
        <div id="schema-content">Loading schema...</div>
      </div>
      <div class="panel" id="docs-panel" style="display: none;">
        <div class="docs-section">
          <h2>Getting Started</h2>
          <p>Write your GraphQL query in the editor and click "Run" to execute it.</p>
        </div>
        <div class="docs-section">
          <h2>Example Query</h2>
          <p>List all instances:</p>
          <pre style="background: #0f3460; padding: 12px; border-radius: 4px; margin-top: 8px;">query {
  instances {
    items { id }
    total
  }
}</pre>
        </div>
      </div>
    </div>
    <div class="main">
      <div class="toolbar">
        <button class="btn" id="run-btn">▶ Run</button>
        <span style="color: #888; font-size: 13px;">Endpoint: <code>%s</code></span>
      </div>
      <div class="editor-container">
        <div class="editor">
          <div class="editor-header">Query</div>
          <textarea id="query" placeholder="Enter your GraphQL query here...">query {
  __schema {
    queryType { name }
    mutationType { name }
  }
}</textarea>
        </div>
        <div class="results">
          <div class="results-header">Response</div>
          <pre id="results">Click "Run" to execute query</pre>
        </div>
      </div>
    </div>
  </div>

  <script>
    const endpoint = '%s';
    const queryEl = document.getElementById('query');
    const resultsEl = document.getElementById('results');
    const runBtn = document.getElementById('run-btn');
    const schemaContent = document.getElementById('schema-content');

    // Tab switching
    document.querySelectorAll('.tab').forEach(tab => {
      tab.addEventListener('click', () => {
        document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        const tabName = tab.dataset.tab;
        document.getElementById('schema-panel').style.display = tabName === 'schema' ? 'block' : 'none';
        document.getElementById('docs-panel').style.display = tabName === 'docs' ? 'block' : 'none';
      });
    });

    // Run query
    async function runQuery() {
      runBtn.disabled = true;
      runBtn.textContent = '⏳ Running...';

      try {
        const response = await fetch(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: queryEl.value })
        });
        const data = await response.json();
        resultsEl.textContent = JSON.stringify(data, null, 2);
      } catch (err) {
        resultsEl.textContent = 'Error: ' + err.message;
      } finally {
        runBtn.disabled = false;
        runBtn.textContent = '▶ Run';
      }
    }

    runBtn.addEventListener('click', runQuery);
    queryEl.addEventListener('keydown', (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        runQuery();
      }
    });

    // Load and display schema
    async function loadSchema() {
      try {
        const response = await fetch(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: '{ __schema { types { name kind fields { name type { name kind ofType { name kind } } } } } }' })
        });
        const data = await response.json();

        if (data.data && data.data.__schema) {
          const types = data.data.__schema.types.filter(t =>
            !t.name.startsWith('__') &&
            !['String', 'Int', 'Float', 'Boolean', 'ID', 'JSON', 'Time'].includes(t.name)
          );

          let html = '';
          types.forEach(type => {
            html += '<div class="schema-type">';
            html += '<h3>' + type.name + ' <span style="color:#888;font-weight:normal;">(' + type.kind + ')</span></h3>';
            if (type.fields) {
              type.fields.forEach(field => {
                const typeName = field.type.name || (field.type.ofType ? field.type.ofType.name : '?');
                html += '<div class="schema-field"><span class="field-name">' + field.name + '</span>: <span class="field-type">' + typeName + '</span></div>';
              });
            }
            html += '</div>';
          });
          schemaContent.innerHTML = html || '<p style="color:#888;">No types found</p>';
        }
      } catch (err) {
        schemaContent.innerHTML = '<p style="color:#e94560;">Error loading schema: ' + err.message + '</p>';
      }
    }

    loadSchema();
  </script>
</body>
</html>`, graphqlPath, graphqlPath)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	}
}
