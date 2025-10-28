Awesome — thanks for the skeleton. Below is a drop-in frontend plus **only** the backend changes needed **inside `app/`** (and a new `public/` folder). It’s Tailwind + vanilla ES modules with your JS style rules (semicolons, `function` notation, braces, spacing, `import`).

---

# New/Updated Backend (only in `app/`)

## app/app.go

```go
package app

import (
	"log"
	"net/http"
	"path/filepath"

	"opnlaas/auth"
)

type App struct {
	mux    *http.ServeMux
	views  *Views
	static http.Handler
}

func New() *App {
	a := &App{
		mux:   http.NewServeMux(),
		views: NewViews(),
		// Serve ./public at /public/
		static: http.StripPrefix("/public/", http.FileServer(http.Dir(filepath.Join("public")))),
	}

	// Public pages
	a.mux.HandleFunc("/", a.handleLanding())
	a.mux.HandleFunc("/login", a.handleLoginPage())
	a.mux.HandleFunc("/logout", a.handleLogout())

	// Protected page
	a.mux.Handle("/dashboard", RequireAuth(http.HandlerFunc(a.handleDashboard())))

	// Static
	a.mux.Handle("/public/", a.static)

	// JSON API (public/minimal)
	a.mux.HandleFunc("/api/login", a.apiLogin())
	a.mux.HandleFunc("/api/logout", a.apiLogout())
	a.mux.Handle("/api/me", RequireAuth(http.HandlerFunc(a.apiMe())))
	a.mux.HandleFunc("/api/resources", a.apiListPublicResources())

	// Admin/host mgmt APIs
	a.mux.Handle("/api/admin/vendors", RequireAdmin(http.HandlerFunc(a.apiVendors())))
	a.mux.Handle("/api/admin/formfactors", RequireAdmin(http.HandlerFunc(a.apiFormFactors())))
	a.mux.Handle("/api/admin/hosts", RequireAdmin(http.HandlerFunc(a.apiHostsCreate())))
	a.mux.Handle("/api/admin/hosts/power", RequireAdmin(http.HandlerFunc(a.apiHostPower())))
	a.mux.Handle("/api/admin/hosts/refresh", RequireAdmin(http.HandlerFunc(a.apiHostRefresh())))
	a.mux.Handle("/api/admin/hosts/pxe", RequireAdmin(http.HandlerFunc(a.apiHostPXE())))

	return a
}

func (a *App) Handler() http.Handler { return a.mux }

// ---- Page Handlers ----

func (a *App) handleLanding() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = a.views.Render(w, r, "landing.html", map[string]any{
			"Title": "Open Lab as a Service",
			"User":  auth.CurrentUser(r.Context()), // may be nil
		})
	}
}

func (a *App) handleLoginPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = a.views.Render(w, r, "login.html", map[string]any{
			"Title": "Log in",
		})
	}
}

func (a *App) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Do a server-side logout if your auth layer supports it.
		// Otherwise the /api/logout endpoint plus JS is enough.
		_ = a.views.Render(w, r, "logout.html", map[string]any{
			"Title": "Logged out",
		})
	}
}

func (a *App) handleDashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.RequireUser(r.Context())
		_ = a.views.Render(w, r, "dashboard.html", map[string]any{
			"Title": "Dashboard",
			"User":  user,
			"IsAdmin": user != nil && user.IsAdmin, // assumes your auth.User has IsAdmin
		})
	}
}
```

## app/middleware.go

```go
package app

import (
	"context"
	"net/http"

	"opnlaas/auth"
)

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := auth.CurrentUser(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user)))
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := auth.RequireUser(r.Context())
		if !user.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
```

> Assumptions:
>
> * `auth.CurrentUser(ctx)` returns `*auth.User` or `nil`.
> * `auth.RequireUser(ctx)` panics or returns a non-nil `*auth.User` (whichever your auth framework prefers).
> * `auth.User` has at least `Username string` and `IsAdmin bool`.
> * If your keys differ, tweak in one spot above.

## app/views.go

```go
package app

import (
	"html/template"
	"net/http"
	"path/filepath"
)

type Views struct {
	t *template.Template
}

func NewViews() *Views {
	funcs := template.FuncMap{
		"safe": func(s string) template.HTML { return template.HTML(s) },
	}
	t := template.Must(template.New("base.html").Funcs(funcs).ParseFiles(
		filepath.Join("app", "templates", "base.html"),
		filepath.Join("app", "templates", "landing.html"),
		filepath.Join("app", "templates", "login.html"),
		filepath.Join("app", "templates", "logout.html"),
		filepath.Join("app", "templates", "dashboard.html"),
	))
	return &Views{t: t}
}

func (v *Views) Render(w http.ResponseWriter, r *http.Request, name string, data any) error {
	return v.t.ExecuteTemplate(w, name, data)
}
```

## app/api.go

```go
package app

import (
	"encoding/json"
	"net/http"

	"opnlaas/auth"
	"opnlaas/hosts"
)

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResp struct{ Error string `json:"error"` }

func badRequest(w http.ResponseWriter, msg string) { writeJSON(w, http.StatusBadRequest, errorResp{Error: msg}) }

// --- Auth ---

func (a *App) apiLogin() http.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			badRequest(w, "POST required")
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid json")
			return
		}
		user, err := auth.Login(r.Context(), body.Username, body.Password)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "invalid credentials"})
			return
		}
		// If your auth sets cookies itself, nothing else to do here.
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{"username": user.Username, "isAdmin": user.IsAdmin},
		})
	}
}

func (a *App) apiLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = auth.Logout(w, r) // if supported; otherwise, clear auth cookie here
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (a *App) apiMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.RequireUser(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{"username": user.Username, "isAdmin": user.IsAdmin},
		})
	}
}

// --- Public resource list (read-only) ---

func (a *App) apiListPublicResources() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Implement a "safe" list in your hosts package; or filter here.
		list, err := hosts.ListPublic(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"resources": list})
	}
}

// --- Admin endpoints ---

func (a *App) apiVendors() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Expect your hosts package to expose vendor/form-factor maps/keys
		writeJSON(w, http.StatusOK, map[string]any{
			"vendors": hosts.VendorKeys(), // e.g., derive from a map[string]VendorSpec
		})
	}
}

func (a *App) apiFormFactors() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"formFactors": hosts.FormFactorKeys(),
		})
	}
}

func (a *App) apiHostsCreate() http.HandlerFunc {
	type req struct {
		ManagementIP string `json:"managementIP"`
		Vendor       string `json:"vendor"`
		FormFactor   string `json:"formFactor"`
		MgmtType     string `json:"managementType"` // "Redfish" | "IPMI"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			badRequest(w, "POST required")
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid json")
			return
		}
		h, err := hosts.Create(r.Context(), hosts.CreateParams{
			ManagementIP: body.ManagementIP,
			Vendor:       body.Vendor,
			FormFactor:   body.FormFactor,
			MgmtType:     body.MgmtType,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"host": h})
	}
}

func (a *App) apiHostPower() http.HandlerFunc {
	type req struct {
		HostID string `json:"hostId"`
		Action string `json:"action"` // "on" | "off" | "cycle" | "reset"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			badRequest(w, "POST required")
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid json")
			return
		}
		if err := hosts.PowerAction(r.Context(), body.HostID, body.Action); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (a *App) apiHostRefresh() http.HandlerFunc {
	type req struct{ HostID string `json:"hostId"` }
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			badRequest(w, "POST required")
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid json")
			return
		}
		if err := hosts.RefreshSpecs(r.Context(), body.HostID); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (a *App) apiHostPXE() http.HandlerFunc {
	type req struct {
		HostID string `json:"hostId"`
		Target string `json:"target"` // e.g., "network", "disk", specific label
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			badRequest(w, "POST required")
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			badRequest(w, "invalid json")
			return
		}
		if err := hosts.SetPXE(r.Context(), body.HostID, body.Target); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
```

> The `hosts.*` calls reference functions you’ve hinted exist (CRUD, IPMI/Redfish). If your names differ, adjust those calls; nothing else in the app changes.

---

# Templates (Tailwind + ES Modules)

Create `app/templates/` and add:

## app/templates/base.html

```html
{{define "base.html"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <!-- Tailwind CDN (no build step) -->
  <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="min-h-screen bg-slate-50 text-slate-900">
  <header class="border-b bg-white">
    <div class="max-w-6xl mx-auto px-4 py-3 flex items-center justify-between">
      <a href="/" class="font-semibold text-lg">OPN-LAAS</a>
      <nav class="flex items-center gap-4">
        <a class="text-sm hover:underline" href="/">Home</a>
        <a class="text-sm hover:underline" href="/dashboard">Dashboard</a>
        {{if .User}}
          <span class="text-sm text-slate-500">Hello, {{.User.Username}}</span>
          <button id="logoutBtn" class="text-sm px-3 py-1 rounded bg-slate-900 text-white">Logout</button>
        {{else}}
          <a class="text-sm px-3 py-1 rounded bg-slate-900 text-white" href="/login">Login</a>
        {{end}}
      </nav>
    </div>
  </header>

  <main class="max-w-6xl mx-auto px-4 py-8">
    {{template "content" .}}
  </main>

  <div id="toast" class="hidden fixed bottom-4 right-4 px-4 py-2 rounded bg-black text-white text-sm"></div>

  <script type="module" src="/public/js/boot.js"></script>
</body>
</html>
{{end}}
```

## app/templates/landing.html

```html
{{define "landing.html"}}{{template "base.html" .}}{{end}}
{{define "content"}}
<section class="mb-10">
  <h1 class="text-3xl font-bold mb-2">Welcome to Open Lab as a Service</h1>
  <p class="text-slate-600">Browse available lab resources. Log in for more controls.</p>
</section>

<section>
  <h2 class="text-xl font-semibold mb-4">Available Resources</h2>
  <div id="resourceGrid" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
    <!-- Filled by /public/js/landing.js -->
  </div>
</section>

<script type="module">
  import { loadLanding } from "/public/js/landing.js";
  loadLanding();
</script>
{{end}}
```

## app/templates/login.html

```html
{{define "login.html"}}{{template "base.html" .}}{{end}}
{{define "content"}}
<div class="max-w-md mx-auto bg-white border rounded-lg p-6">
  <h1 class="text-2xl font-semibold mb-4">Log in</h1>
  <form id="loginForm" class="space-y-4">
    <div>
      <label class="block text-sm mb-1">Username</label>
      <input type="text" name="username" class="w-full border rounded px-3 py-2" required>
    </div>
    <div>
      <label class="block text-sm mb-1">Password</label>
      <input type="password" name="password" class="w-full border rounded px-3 py-2" required>
    </div>
    <button class="w-full bg-slate-900 text-white rounded py-2">Log in</button>
  </form>
</div>

<script type="module">
  import { attachLogin } from "/public/js/login.js";
  attachLogin();
</script>
{{end}}
```

## app/templates/logout.html

```html
{{define "logout.html"}}{{template "base.html" .}}{{end}}
{{define "content"}}
<div class="max-w-md mx-auto bg-white border rounded-lg p-6 text-center">
  <h1 class="text-2xl font-semibold mb-2">You’ve been logged out</h1>
  <p class="text-slate-600 mb-4">See you next time.</p>
  <a href="/" class="px-4 py-2 rounded bg-slate-900 text-white">Go home</a>
</div>
{{end}}
```

## app/templates/dashboard.html

```html
{{define "dashboard.html"}}{{template "base.html" .}}{{end}}
{{define "content"}}
<h1 class="text-2xl font-semibold mb-6">Dashboard</h1>

<div id="userInfo" class="mb-6 text-sm text-slate-600"></div>

<section class="mb-10">
  <h2 class="text-xl font-semibold mb-3">Resources</h2>
  <div class="overflow-x-auto bg-white border rounded-lg">
    <table class="min-w-full text-sm">
      <thead class="bg-slate-50">
        <tr>
          <th class="text-left px-3 py-2">Name</th>
          <th class="text-left px-3 py-2">Vendor</th>
          <th class="text-left px-3 py-2">Form Factor</th>
          <th class="text-left px-3 py-2">Mgmt</th>
          <th class="text-left px-3 py-2">Mgmt IP</th>
          <th class="text-left px-3 py-2">Power</th>
          <th class="text-left px-3 py-2">Actions</th>
        </tr>
      </thead>
      <tbody id="hostRows"></tbody>
    </table>
  </div>
</section>

{{if .IsAdmin}}
<section class="mb-10">
  <h2 class="text-xl font-semibold mb-3">Add Host</h2>
  <form id="addHostForm" class="grid grid-cols-1 md:grid-cols-2 gap-4 bg-white border rounded-lg p-4">
    <div>
      <label class="block text-sm mb-1">Management IP</label>
      <input name="managementIP" class="w-full border rounded px-3 py-2" placeholder="10.0.0.10" required>
    </div>
    <div>
      <label class="block text-sm mb-1">Vendor</label>
      <select name="vendor" id="vendorSelect" class="w-full border rounded px-3 py-2" required></select>
    </div>
    <div>
      <label class="block text-sm mb-1">Form Factor</label>
      <select name="formFactor" id="formFactorSelect" class="w-full border rounded px-3 py-2" required></select>
    </div>
    <div>
      <label class="block text-sm mb-1">Management Type</label>
      <select name="managementType" class="w-full border rounded px-3 py-2" required>
        <option>Redfish</option>
        <option>IPMI</option>
      </select>
    </div>
    <div class="md:col-span-2">
      <button class="px-4 py-2 rounded bg-slate-900 text-white">Add Host</button>
    </div>
  </form>
</section>
{{end}}

<script type="module">
  import { loadDashboard } from "/public/js/dashboard.js";
  loadDashboard({ isAdmin: {{if .IsAdmin}}true{{else}}false{{end}} });
</script>
{{end}}
```

---

# New `public/` Folder

```
public/
├── js/
│   ├── api.js
│   ├── boot.js
│   ├── landing.js
│   ├── login.js
│   ├── dashboard.js
│   └── ui.js
└── img/   (optional)
```

## public/js/boot.js

```javascript
// Shared boot for layout-level hooks
import { logout, getMe } from "./api.js";
import { toast } from "./ui.js";

(function initHeader() {
  const btn = document.getElementById("logoutBtn");
  if (!btn) { return; }
  btn.addEventListener("click", async function () {
    try {
      await logout();
      toast("Logged out");
      window.location.href = "/logout";
    } catch (e) {
      toast("Failed to logout");
    }
  });
})();

export async function reflectUser(targetId) {
  try {
    const me = await getMe();
    const el = document.getElementById(targetId);
    if (!el) { return; }
    el.textContent = `Signed in as ${me.user.username} ${me.user.isAdmin ? "(admin)" : ""}`;
  } catch (_e) {
    // not signed in or unreachable
  }
}
```

## public/js/ui.js

```javascript
export function toast(message) {
  const t = document.getElementById("toast");
  if (!t) { return; }
  t.textContent = message;
  t.classList.remove("hidden");
  setTimeout(function () {
    t.classList.add("hidden");
  }, 2100);
}

export function el(tag, attrs, children) {
  const e = document.createElement(tag);
  if (attrs) {
    Object.keys(attrs).forEach(function (k) {
      if (k === "class") { e.className = attrs[k]; }
      else if (k === "text") { e.textContent = attrs[k]; }
      else { e.setAttribute(k, attrs[k]); }
    });
  }
  if (children && children.length) {
    children.forEach(function (c) { e.appendChild(c); });
  }
  return e;
}
```

## public/js/api.js

```javascript
const JSON_HEADERS = { "Content-Type": "application/json" };

async function apiFetch(path, opts) {
  const res = await fetch(path, Object.assign({ credentials: "include" }, opts || {}));
  if (!res.ok) {
    let msg = "request failed";
    try { const j = await res.json(); msg = j.error || msg; } catch (_) {}
    throw new Error(msg);
  }
  return res.json();
}

// Auth
export async function login(username, password) {
  return apiFetch("/api/login", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ username, password })
  });
}
export async function logout() {
  return apiFetch("/api/logout", { method: "POST" });
}
export async function getMe() {
  return apiFetch("/api/me");
}

// Public resources
export async function listResources() {
  return apiFetch("/api/resources");
}

// Admin dictionaries
export async function getVendors() {
  return apiFetch("/api/admin/vendors");
}
export async function getFormFactors() {
  return apiFetch("/api/admin/formfactors");
}

// Admin host mgmt
export async function addHost(payload) {
  return apiFetch("/api/admin/hosts", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify(payload)
  });
}
export async function hostPower(hostId, action) {
  return apiFetch("/api/admin/hosts/power", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ hostId, action })
  });
}
export async function hostRefresh(hostId) {
  return apiFetch("/api/admin/hosts/refresh", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ hostId })
  });
}
export async function hostPXE(hostId, target) {
  return apiFetch("/api/admin/hosts/pxe", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ hostId, target })
  });
}
```

## public/js/landing.js

```javascript
import { listResources } from "./api.js";
import { el, toast } from "./ui.js";

export async function loadLanding() {
  try {
    const { resources } = await listResources();
    const grid = document.getElementById("resourceGrid");
    if (!grid) { return; }
    grid.innerHTML = "";
    resources.forEach(function (r) {
      const card = el("div", { class: "border bg-white rounded-lg p-4" }, [
        el("div", { class: "font-semibold text-lg", text: r.name || r.host || "Unnamed" }),
        el("div", { class: "text-sm text-slate-600", text: `${r.vendor || "Unknown"} • ${r.formFactor || "N/A"}` }),
        el("div", { class: "text-sm text-slate-500 mt-1", text: `${r.managementType || "Mgmt"} @ ${r.managementIP || "-"}` })
      ]);
      grid.appendChild(card);
    });
  } catch (e) {
    toast("Failed to load resources");
  }
}
```

## public/js/login.js

```javascript
import { login } from "./api.js";
import { toast } from "./ui.js";

export function attachLogin() {
  const form = document.getElementById("loginForm");
  if (!form) { return; }
  form.addEventListener("submit", async function (ev) {
    ev.preventDefault();
    const data = new FormData(form);
    const username = data.get("username");
    const password = data.get("password");
    try {
      await login(username, password);
      toast("Welcome!");
      window.location.href = "/dashboard";
    } catch (e) {
      toast(e.message || "Login failed");
    }
  });
}
```

## public/js/dashboard.js

```javascript
import { getMe, listResources, addHost, getVendors, getFormFactors, hostPower, hostRefresh, hostPXE } from "./api.js";
import { toast, el } from "./ui.js";
import { reflectUser } from "./boot.js";

export async function loadDashboard(opts) {
  await reflectUser("userInfo");
  await loadHostTable(opts && opts.isAdmin);
  if (opts && opts.isAdmin) {
    await hydrateDictionaries();
    wireAddHostForm();
  }
}

async function hydrateDictionaries() {
  try {
    const [vendors, forms] = await Promise.all([getVendors(), getFormFactors()]);
    fillSelect("vendorSelect", vendors.vendors || []);
    fillSelect("formFactorSelect", (forms.formFactors || []));
  } catch (_e) { /* ignore */ }
}

function fillSelect(id, options) {
  const sel = document.getElementById(id);
  if (!sel) { return; }
  sel.innerHTML = "";
  options.forEach(function (v) {
    const o = document.createElement("option");
    o.value = v; o.textContent = v;
    sel.appendChild(o);
  });
}

async function loadHostTable(isAdmin) {
  const tbody = document.getElementById("hostRows");
  if (!tbody) { return; }
  tbody.innerHTML = "";
  try {
    const { resources } = await listResources();
    resources.forEach(function (h) {
      const tr = document.createElement("tr");
      tr.className = "border-t";

      const power = el("td", { class: "px-3 py-2" }, [
        el("span", { class: "inline-block px-2 py-1 rounded bg-slate-100", text: (h.powerState || "unknown") })
      ]);

      const actions = el("td", { class: "px-3 py-2" });
      if (isAdmin) {
        actions.appendChild(rowActions(h));
      } else {
        actions.textContent = "-";
      }

      tr.appendChild(td(h.name || h.host || "Unnamed"));
      tr.appendChild(td(h.vendor || "-"));
      tr.appendChild(td(h.formFactor || "-"));
      tr.appendChild(td(h.managementType || "-"));
      tr.appendChild(td(h.managementIP || "-"));
      tr.appendChild(power);
      tr.appendChild(actions);
      tbody.appendChild(tr);
    });
  } catch (e) {
    toast("Failed to load hosts");
  }
}

function td(text) {
  return el("td", { class: "px-3 py-2", text: text });
}

function rowActions(h) {
  const wrap = el("div", { class: "flex gap-2 flex-wrap" });

  const mkBtn = function (label, handler) {
    const b = el("button", { class: "text-xs px-2 py-1 rounded bg-slate-900 text-white" , text: label });
    b.addEventListener("click", async function () {
      try {
        await handler();
        toast(label + " OK");
      } catch (e) {
        toast(e.message || (label + " failed"));
      }
    });
    return b;
  };

  wrap.appendChild(mkBtn("Power On", function () { return hostPower(h.id, "on"); }));
  wrap.appendChild(mkBtn("Power Off", function () { return hostPower(h.id, "off"); }));
  wrap.appendChild(mkBtn("Cycle", function () { return hostPower(h.id, "cycle"); }));
  wrap.appendChild(mkBtn("Reset", function () { return hostPower(h.id, "reset"); }));
  wrap.appendChild(mkBtn("Refresh Specs", function () { return hostRefresh(h.id); }));
  // Simple prompt for PXE target
  wrap.appendChild(mkBtn("Set PXE", async function () {
    const t = window.prompt("PXE target (e.g., network, disk, or profile name):");
    if (!t) { return; }
    return hostPXE(h.id, t);
  }));

  return wrap;
}

function wireAddHostForm() {
  const form = document.getElementById("addHostForm");
  if (!form) { return; }
  form.addEventListener("submit", async function (ev) {
    ev.preventDefault();
    const data = new FormData(form);
    const payload = {
      managementIP: data.get("managementIP"),
      vendor: data.get("vendor"),
      formFactor: data.get("formFactor"),
      managementType: data.get("managementType")
    };
    try {
      await addHost(payload);
      toast("Host added");
      form.reset();
      await loadHostTable(true);
    } catch (e) {
      toast(e.message || "Failed to add host");
    }
  });
}
```

---

# Notes on Integration & Expectations

* **Only `app/` changed**, plus new `public/`. No edits to `auth/`, `hosts/`, `config/`, etc.

* The API handlers expect the following lightweight surface in your existing packages (rename if yours differ):

  * `auth.Login(ctx, user, pass) (*auth.User, error)`
  * `auth.Logout(w, r) error` (optional, you can no-op)
  * `auth.CurrentUser(ctx) *auth.User` and `auth.RequireUser(ctx) *auth.User`
  * `type User struct { Username string; IsAdmin bool }`
  * `hosts.ListPublic(ctx) ([]HostPublic, error)` where items include: `id`, `name/host`, `vendor`, `formFactor`, `managementType`, `managementIP`, `powerState`
  * `hosts.VendorKeys() []string`
  * `hosts.FormFactorKeys() []string`
  * `hosts.Create(ctx, CreateParams) (Host, error)`
  * `hosts.PowerAction(ctx, hostID, action string) error`
  * `hosts.RefreshSpecs(ctx, hostID string) error`
  * `hosts.SetPXE(ctx, hostID, target string) error`

* If your DB handler already exposes slightly different names, you only need to tweak the calls in **`app/api.go`** (single file) to match.

* **Routing hookup:** if your `main.go` constructs `app.New()` already, ensure it calls `Handler()` and serves on your existing server. If your main wires routes differently, simply mount `a.Handler()` at your chosen root.

* **Tailwind:** using CDN to avoid a build step; if you want JIT/custom config, you can later drop a build pipeline without changing the templates.

* **Security:** Add CSRF or stricter method checks later if you need; right now, admin endpoints are protected by `RequireAdmin`.

---

If you want me to align the `hosts.*` and `auth.*` calls to your exact public methods, paste those signatures and I’ll wire them precisely.
