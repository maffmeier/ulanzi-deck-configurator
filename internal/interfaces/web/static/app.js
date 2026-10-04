"use strict";

const INFO_INDEX = 13;

// D200: 13 keys on a 5x3 grid, the wide info window takes the last two
// cells of the bottom row.
const SLOTS = Object.freeze([
    ...Array.from({ length: 13 }, (_, i) => ({ index: i, row: Math.floor(i / 5) + 1, column: (i % 5) + 1, span: 1 })),
    { index: INFO_INDEX, row: 3, column: 4, span: 2, kind: "info" },
]);

const ACTION_LABELS = Object.freeze({
    none: "Keine Aktion",
    shell: "Befehl ausführen",
    shortcut: "Tastenkürzel",
    url: "Website öffnen",
    switch_page: "Seite wechseln",
    predefined_command: "Systemfunktion",
});

const ACTION_ICONS = Object.freeze({
    shell: "fa:solid:terminal",
    shortcut: "fa:solid:keyboard",
    url: "fa:solid:globe",
    switch_page: "fa:solid:layer-group",
    predefined_command: "fa:solid:gear",
});

// Built-in actions come from /api/predefined (platform specific); these
// are the generic building blocks shown above them.
const BASE_GROUPS = Object.freeze([
    {
        name: "Allgemein",
        items: [
            { id: "shell", label: "Befehl ausführen", icon: "fa:solid:terminal", hint: "Startet ein Programm oder Shell-Kommando", action: { type: "shell" }, focus: "cmd" },
            { id: "shortcut", label: "Tastenkürzel", icon: "fa:solid:keyboard", hint: "Simuliert eine Tastenkombination", action: { type: "shortcut" }, focus: "keys" },
            { id: "url", label: "Website öffnen", icon: "fa:solid:globe", hint: "Öffnet eine Adresse im Standardbrowser", action: { type: "url" }, focus: "url" },
        ],
    },
    {
        name: "Navigation",
        items: [
            { id: "page-next", label: "Nächste Seite", icon: "fa:solid:arrow-right", hint: "Blättert zur nächsten Seite", action: { type: "switch_page", page: "@next" }, title: "Weiter →" },
            { id: "page-prev", label: "Vorherige Seite", icon: "fa:solid:arrow-left", hint: "Blättert zur vorherigen Seite", action: { type: "switch_page", page: "@prev" }, title: "← Zurück" },
        ],
    },
]);

const FONTS = Object.freeze(["DejaVu Sans", "DejaVu Serif", "DejaVu Sans Mono", "Liberation Sans", "Liberation Serif"]);

const METRICS = Object.freeze([
    { id: "cpu", label: "CPU" },
    { id: "memory", label: "Arbeitsspeicher" },
    { id: "gpu", label: "GPU" },
    { id: "temperature", label: "Temperatur" },
    { id: "disk", label: "Festplatte" },
    { id: "network", label: "Netzwerk" },
    { id: "battery", label: "Akku" },
]);

const DEFAULT_STYLE = Object.freeze({
    background_color: "#111827",
    text_color: "#F8FAFC",
    bold: false,
    italic: false,
    underline: false,
    font_family: "DejaVu Sans",
    font_size: 30,
});

const KEY_NAMES = Object.freeze({
    " ": "space", Enter: "Return", Escape: "Escape", Tab: "Tab", Backspace: "BackSpace", Delete: "Delete",
    Insert: "Insert", Home: "Home", End: "End", PageUp: "Prior", PageDown: "Next",
    ArrowUp: "Up", ArrowDown: "Down", ArrowLeft: "Left", ArrowRight: "Right", PrintScreen: "Print",
    MediaPlayPause: "XF86AudioPlay", MediaTrackNext: "XF86AudioNext", MediaTrackPrevious: "XF86AudioPrev",
    AudioVolumeUp: "XF86AudioRaiseVolume", AudioVolumeDown: "XF86AudioLowerVolume", AudioVolumeMute: "XF86AudioMute",
});

function emptyAction() {
    return { type: "none", cmd: "", keys: "", command_id: "", url: "", page: "" };
}

function normalizeButton(raw) {
    return {
        index: raw.index,
        label: raw.label || "",
        icon_path: raw.icon_path || "",
        preview_url: raw.preview_url || "",
        action: { ...emptyAction(), ...(raw.action || {}) },
        text_style: { ...DEFAULT_STYLE, ...(raw.text_style || {}) },
    };
}

function isEmptyButton(b) {
    return !b.label && !b.icon_path && (!b.action || b.action.type === "none") &&
        JSON.stringify({ ...DEFAULT_STYLE, ...b.text_style }).toLowerCase() === JSON.stringify(DEFAULT_STYLE).toLowerCase();
}

function iconUrl(assetId) {
    return assetId ? `/api/builtin-asset?asset_id=${encodeURIComponent(assetId)}` : "";
}

window.deckApp = function deckApp() {
    return {
        INFO_INDEX, ACTION_LABELS, FONTS, METRICS,
        predefined: [],
        slots: SLOTS,
        editor: null,
        currentPage: "",
        selected: null,
        dirty: false,
        busy: false,
        saveBundle: false,
        health: { deck_connected: false },
        preview: { time_text: "--:--", cpu_percent: 0, mem_percent: 0, metrics: [] },
        sensors: [],
        actionQuery: "",
        dragOverKey: null,
        dragOverPage: null,
        recording: false,
        toasts: [],
        catalog: { open: false, loaded: false, items: [], query: "", style: "all" },
        shellPlaceholder: navigator.platform.startsWith("Win") ? "z.B. start notepad" : "z.B. swaymsg exec alacritty",

        async init() {
            await Promise.all([this.load(), this.loadPredefined()]);
            this.pollHealth();
            this.pollPreview();
            this.loadSensors();
            window.addEventListener("beforeunload", (e) => {
                if (this.dirty) {
                    e.preventDefault();
                }
            });
        },

        // ───────────── Laden & Speichern ─────────────

        async load() {
            this.busy = true;
            try {
                const res = await fetch("/api/editor");
                const payload = await res.json();
                if (!res.ok) {
                    throw new Error(payload.detail || "Konfiguration konnte nicht geladen werden");
                }
                this.editor = this.normalizeEditor(payload);
                if (!this.editor.pages.some((p) => p.name === this.currentPage)) {
                    this.currentPage = this.editor.default_page;
                }
                this.selected = null;
                this.dirty = false;
            } catch (err) {
                this.toast(err.message, "err");
            } finally {
                this.busy = false;
            }
        },

        normalizeEditor(payload) {
            const sw = payload.small_window || {};
            return {
                path: payload.path,
                default_page: payload.default_page,
                brightness: Number.isFinite(payload.brightness) ? payload.brightness : 50,
                pages: (payload.pages || []).map((p) => ({ name: p.name, buttons: (p.buttons || []).map(normalizeButton) })),
                fixed_buttons: (payload.fixed_buttons || []).map(normalizeButton),
                small_window: {
                    enabled: Boolean(sw.enabled),
                    interval_s: sw.interval_s || 2,
                    time_format: sw.time_format || "%H:%M",
                    show_metrics: sw.show_metrics !== false,
                    rotate_every_s: sw.rotate_every_s ?? null,
                    background_color: sw.background_color || "#000000",
                    metrics_items: sw.metrics_items || [],
                    temperature_sensors: sw.temperature_sensors || [],
                    temperature_separator: sw.temperature_separator === "|" ? "|" : " ",
                },
            };
        },

        async reload() {
            if (this.dirty && !confirm("Ungespeicherte Änderungen verwerfen?")) {
                return;
            }
            await this.load();
            this.toast("Änderungen verworfen");
        },

        serializeButtons(buttons) {
            return buttons.filter((b) => !isEmptyButton(b)).map((b) => ({
                index: b.index,
                label: b.label,
                icon_path: b.icon_path || null,
                action: b.action,
                text_style: { ...b.text_style, font_size: Number(b.text_style.font_size) },
            }));
        },

        payload() {
            const sw = this.editor.small_window;
            return {
                default_page: this.editor.default_page,
                brightness: Number(this.editor.brightness),
                pages: this.editor.pages.map((p) => ({ name: p.name, buttons: this.serializeButtons(p.buttons) })),
                fixed_buttons: this.serializeButtons(this.editor.fixed_buttons),
                small_window: {
                    ...sw,
                    interval_s: Number(sw.interval_s),
                    rotate_every_s: sw.rotate_every_s === null || sw.rotate_every_s === "" ? null : Number(sw.rotate_every_s),
                },
                save_firmware_bundle: this.saveBundle,
            };
        },

        async save() {
            this.busy = true;
            try {
                const res = await fetch("/api/editor", {
                    method: "PUT",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(this.payload()),
                });
                const payload = await res.json();
                if (!res.ok) {
                    throw new Error(payload.error || payload.detail || "Speichern fehlgeschlagen");
                }
                const selected = this.selected;
                this.editor = this.normalizeEditor(payload);
                this.dirty = false;
                this.selected = selected;
                this.toast(this.health.deck_connected ? "Gespeichert – das Deck übernimmt die Änderung gleich" : "Gespeichert", "ok");
            } catch (err) {
                this.toast(err.message, "err", 8000);
            } finally {
                this.busy = false;
            }
        },

        markDirty() {
            this.dirty = true;
        },

        // Selects show their first option when the bound value is empty, so
        // give every action a real default instead of a misleading blank.
        actionTypeChanged() {
            const a = this.cur.action;
            if (a.type === "switch_page" && !a.page) a.page = "@next";
            if (a.type === "predefined_command" && !a.command_id) a.command_id = this.predefined[0]?.id || "";
            this.markDirty();
        },

        // ───────────── Status ─────────────

        async pollHealth() {
            try {
                const res = await fetch("/api/health");
                this.health = await res.json();
            } catch (_err) {
                this.health = { deck_connected: false };
            }
            setTimeout(() => this.pollHealth(), 3000);
        },

        async pollPreview() {
            if (this.editor) {
                const sw = this.editor.small_window;
                const params = new URLSearchParams([["time_format", sw.time_format || "%H:%M"]]);
                sw.metrics_items.forEach((m) => params.append("metrics_items", m));
                sw.temperature_sensors.forEach((s) => params.append("temperature_sensors", s));
                params.append("temperature_separator", sw.temperature_separator);
                try {
                    const res = await fetch(`/api/small-window/preview?${params}`);
                    if (res.ok) {
                        this.preview = await res.json();
                    }
                } catch (_err) { /* Vorschau ist optional */ }
            }
            setTimeout(() => this.pollPreview(), 2000);
        },

        async loadPredefined() {
            try {
                const res = await fetch("/api/predefined");
                this.predefined = (await res.json()).items || [];
            } catch (_err) {
                this.predefined = [];
            }
        },

        predefinedGroups() {
            const groups = [];
            for (const p of this.predefined) {
                let g = groups.find((x) => x.name === p.group);
                if (!g) groups.push(g = { name: p.group, items: [] });
                g.items.push(p);
            }
            return groups;
        },

        async loadSensors() {
            try {
                const res = await fetch("/api/temperature-sensors");
                this.sensors = (await res.json()).items || [];
            } catch (_err) {
                this.sensors = [];
            }
        },

        toast(text, kind = "", ms = 3500) {
            const id = Date.now() + Math.random();
            this.toasts.push({ id, text, kind });
            setTimeout(() => { this.toasts = this.toasts.filter((t) => t.id !== id); }, ms);
        },

        // ───────────── Seiten ─────────────

        page() {
            return this.editor.pages.find((p) => p.name === this.currentPage);
        },

        pageNames() {
            return this.editor ? this.editor.pages.map((p) => p.name) : [];
        },

        selectPage(name) {
            this.currentPage = name;
            if (this.selected !== null && !this.isFixed(this.selected)) {
                this.selected = null;
            }
        },

        uniquePageName(base) {
            let name = base;
            for (let i = 2; this.pageNames().includes(name); i++) {
                name = `${base} ${i}`;
            }
            return name;
        },

        addPage() {
            const name = this.uniquePageName(`Seite ${this.editor.pages.length + 1}`);
            this.editor.pages.push({ name, buttons: [] });
            this.currentPage = name;
            this.selected = null;
            this.markDirty();
        },

        renamePage(oldName) {
            const name = (prompt("Neuer Name der Seite:", oldName) || "").trim();
            if (!name || name === oldName) {
                return;
            }
            if (this.pageNames().includes(name)) {
                this.toast(`Die Seite „${name}“ gibt es schon`, "err");
                return;
            }
            this.editor.pages.find((p) => p.name === oldName).name = name;
            const retarget = (b) => { if (b.action.type === "switch_page" && b.action.page === oldName) b.action.page = name; };
            this.editor.pages.forEach((p) => p.buttons.forEach(retarget));
            this.editor.fixed_buttons.forEach(retarget);
            if (this.editor.default_page === oldName) this.editor.default_page = name;
            if (this.currentPage === oldName) this.currentPage = name;
            this.markDirty();
        },

        movePage(delta) {
            const pages = this.editor.pages;
            const i = pages.findIndex((p) => p.name === this.currentPage);
            const j = i + delta;
            if (j < 0 || j >= pages.length) return;
            [pages[i], pages[j]] = [pages[j], pages[i]];
            this.markDirty();
        },

        removePage() {
            if (this.editor.pages.length <= 1) return;
            const name = this.currentPage;
            if (this.page().buttons.some((b) => !isEmptyButton(b)) && !confirm(`Seite „${name}“ mit allen Tasten löschen?`)) {
                return;
            }
            this.editor.pages = this.editor.pages.filter((p) => p.name !== name);
            if (this.editor.default_page === name) this.editor.default_page = this.editor.pages[0].name;
            this.currentPage = this.editor.pages[0].name;
            this.selected = null;
            this.markDirty();
        },

        // ───────────── Tasten ─────────────

        isFixed(index) {
            return this.editor.fixed_buttons.some((b) => b.index === index);
        },

        button(index) {
            if (!this.editor) return null;
            return this.editor.fixed_buttons.find((b) => b.index === index)
                || this.page()?.buttons.find((b) => b.index === index)
                || null;
        },

        ensureButton(index) {
            let b = this.button(index);
            if (!b) {
                b = normalizeButton({ index });
                this.page().buttons.push(b);
                b = this.page().buttons[this.page().buttons.length - 1];
            }
            return b;
        },

        get cur() {
            return this.selected === null ? null : this.ensureButton(this.selected);
        },

        select(index) {
            this.selected = index;
        },

        keyClasses(slot) {
            const b = this.button(slot.index);
            return {
                info: slot.kind === "info",
                selected: this.selected === slot.index,
                empty: slot.kind !== "info" && (!b || (!b.preview_url && !b.label)),
                "drop-target": this.dragOverKey === slot.index,
            };
        },

        keyTooltip(slot) {
            const b = this.button(slot.index);
            const name = slot.kind === "info" ? "Infofenster" : `Taste ${slot.index + 1}`;
            if (!b || b.action.type === "none") return name;
            return `${name} · ${this.describeAction(b.action)}`;
        },

        describeAction(a) {
            switch (a.type) {
                case "shell": return `Befehl: ${a.cmd}`;
                case "shortcut": return `Kürzel: ${a.keys}`;
                case "url": return `Website: ${a.url}`;
                case "switch_page": return a.page === "@next" ? "Nächste Seite" : a.page === "@prev" ? "Vorherige Seite" : `Seite: ${a.page}`;
                case "predefined_command": return this.predefined.find((p) => p.id === a.command_id)?.label || a.command_id;
            }
            return "";
        },

        actionIconUrl(action) {
            if (!action) return "";
            if (action.type === "predefined_command") {
                return iconUrl(this.predefined.find((p) => p.id === action.command_id)?.icon || ACTION_ICONS.predefined_command);
            }
            if (action.type === "switch_page") {
                return iconUrl(action.page === "@prev" ? "fa:solid:arrow-left" : action.page === "@next" ? "fa:solid:arrow-right" : "fa:solid:layer-group");
            }
            return iconUrl(ACTION_ICONS[action.type]);
        },

        iconUrl,

        faceStyle(b) {
            if (!b || b.preview_url) return "";
            return `background:${b.text_style.background_color}`;
        },

        // Mirrors the device renderer: the label is scaled from the 196px tile.
        labelStyle(b) {
            const s = b.text_style;
            return [
                `color:${s.text_color}`,
                `font-family:'${s.font_family}', ${s.font_family.includes("Serif") ? "serif" : s.font_family.includes("Mono") ? "monospace" : "sans-serif"}`,
                `font-size:calc(var(--key) * ${s.font_size / 196})`,
                `font-weight:${s.bold ? 700 : 400}`,
                `font-style:${s.italic ? "italic" : "normal"}`,
                `text-decoration:${s.underline ? "underline" : "none"}`,
            ].join(";");
        },

        clearSelected() {
            const index = this.selected;
            this.page().buttons = this.page().buttons.filter((b) => b.index !== index);
            this.editor.fixed_buttons = this.editor.fixed_buttons.filter((b) => b.index !== index);
            this.markDirty();
        },

        toggleFixed() {
            const index = this.selected;
            const b = this.ensureButton(index);
            if (this.isFixed(index)) {
                this.editor.fixed_buttons = this.editor.fixed_buttons.filter((x) => x.index !== index);
                this.page().buttons.push(b);
            } else {
                const others = this.editor.pages.filter((p) => p.name !== this.currentPage && p.buttons.some((x) => x.index === index && !isEmptyButton(x)));
                if (others.length && !confirm(`Taste ${index + 1} ist auf ${others.map((p) => `„${p.name}“`).join(", ")} belegt. Diese Belegung wird ersetzt. Fortfahren?`)) {
                    return;
                }
                this.editor.pages.forEach((p) => { p.buttons = p.buttons.filter((x) => x.index !== index); });
                this.editor.fixed_buttons.push(b);
            }
            this.markDirty();
        },

        onKey(event) {
            if (this.recording) {
                this.captureShortcut(event);
                return;
            }
            const typing = ["INPUT", "SELECT", "TEXTAREA"].includes(event.target.tagName);
            if ((event.ctrlKey || event.metaKey) && event.key === "s") {
                event.preventDefault();
                if (this.dirty) this.save();
            } else if (!typing && (event.key === "Delete" || event.key === "Backspace") && this.selected !== null) {
                event.preventDefault();
                this.clearSelected();
            } else if (!typing && event.key === "Escape") {
                this.selected = null;
            }
        },

        recordShortcut() {
            this.recording = !this.recording;
        },

        captureShortcut(event) {
            event.preventDefault();
            if (["Control", "Alt", "Shift", "Meta", "OS"].includes(event.key)) return;
            const mods = [];
            if (event.ctrlKey) mods.push("ctrl");
            if (event.altKey) mods.push("alt");
            if (event.shiftKey) mods.push("shift");
            if (event.metaKey) mods.push("super");
            let key = KEY_NAMES[event.key] || event.key;
            if (/^F\d+$/.test(event.key)) key = event.key;
            else if (event.code.startsWith("Key")) key = event.code.slice(3).toLowerCase();
            else if (event.code.startsWith("Digit")) key = event.code.slice(5);
            this.cur.action.keys = [...mods, key].join("+");
            this.recording = false;
            this.markDirty();
        },

        // ───────────── Aktionsliste & Drag-and-Drop ─────────────

        filteredActionGroups() {
            const q = this.actionQuery.trim().toLowerCase();
            const pageItems = this.pageNames().map((name) => ({
                id: `page:${name}`, label: `Zu „${name}“`, icon: "fa:solid:layer-group",
                hint: `Springt zur Seite ${name}`, action: { type: "switch_page", page: name }, title: name,
            }));
            const groups = [
                BASE_GROUPS[0],
                { ...BASE_GROUPS[1], items: [...BASE_GROUPS[1].items, ...pageItems] },
                ...this.predefinedGroups().map((g) => ({
                    name: g.name,
                    items: g.items.map((p) => ({
                        id: p.id, label: p.label, icon: p.icon, hint: `${g.name}: ${p.label}`,
                        action: { type: "predefined_command", command_id: p.id }, useIcon: true,
                    })),
                })),
            ];
            if (!q) return groups;
            return groups
                .map((g) => ({ ...g, items: g.items.filter((i) => `${i.label} ${i.hint}`.toLowerCase().includes(q)) }))
                .filter((g) => g.items.length);
        },

        findPreset(id) {
            for (const g of this.filteredActionGroups()) {
                const hit = g.items.find((i) => i.id === id);
                if (hit) return hit;
            }
            return null;
        },

        dragPreset(event, preset) {
            event.dataTransfer.setData("application/x-deck-action", preset.id);
            event.dataTransfer.effectAllowed = "copy";
        },

        dragKey(event, index) {
            event.dataTransfer.setData("application/x-deck-key", String(index));
            event.dataTransfer.effectAllowed = "move";
        },

        async dropOnKey(event, index) {
            this.dragOverKey = null;
            const dt = event.dataTransfer;
            const presetId = dt.getData("application/x-deck-action");
            const keyIndex = dt.getData("application/x-deck-key");
            if (presetId) {
                const preset = this.findPreset(presetId);
                if (preset) await this.applyPreset(index, preset);
            } else if (keyIndex !== "") {
                this.moveKey(Number(keyIndex), index);
            } else if (dt.files?.length) {
                if (index === INFO_INDEX) {
                    this.toast("Das Infofenster kann kein Bild anzeigen", "err");
                    return;
                }
                this.selected = index;
                await this.uploadIcon(dt.files[0]);
            }
        },

        dropOnPage(event, pageName) {
            this.dragOverPage = null;
            const keyIndex = event.dataTransfer.getData("application/x-deck-key");
            if (keyIndex === "" || pageName === this.currentPage) return;
            const index = Number(keyIndex);
            if (this.isFixed(index)) {
                this.toast("Diese Taste ist ohnehin auf allen Seiten", "err");
                return;
            }
            const source = this.page().buttons.find((b) => b.index === index);
            if (!source) return;
            const target = this.editor.pages.find((p) => p.name === pageName);
            const copy = JSON.parse(JSON.stringify(source));
            target.buttons = target.buttons.filter((b) => b.index !== index);
            target.buttons.push(copy);
            this.toast(`Taste ${index + 1} nach „${pageName}“ kopiert`, "ok");
            this.markDirty();
        },

        moveKey(from, to) {
            if (from === to) return;
            if (from === INFO_INDEX || to === INFO_INDEX) {
                this.toast("Das Infofenster lässt sich nicht verschieben", "err");
                return;
            }
            const fromFixed = this.isFixed(from);
            if (fromFixed !== this.isFixed(to) && this.button(to)) {
                this.toast("Feste und seitenbezogene Tasten lassen sich nicht tauschen", "err");
                return;
            }
            const list = fromFixed ? this.editor.fixed_buttons : this.page().buttons;
            const a = list.find((b) => b.index === from);
            const b = list.find((b) => b.index === to);
            if (a) a.index = to;
            if (b) b.index = from;
            this.selected = to;
            this.markDirty();
        },

        applyPresetToSelected(preset) {
            if (this.selected === null) {
                this.toast("Wähle zuerst eine Taste aus", "err");
                return;
            }
            this.applyPreset(this.selected, preset);
        },

        async applyPreset(index, preset) {
            const b = this.ensureButton(index);
            b.action = { ...emptyAction(), ...preset.action };
            if (index !== INFO_INDEX && !b.icon_path && !b.label) {
                if (preset.useIcon) {
                    await this.importIcon(b, preset.icon);
                } else if (preset.title) {
                    b.label = preset.title;
                }
            }
            this.selected = index;
            this.markDirty();
            if (preset.focus) {
                this.$nextTick(() => document.querySelector(".props .mono, .props input[type=url]")?.focus());
            }
        },

        // ───────────── Bilder ─────────────

        async uploadIcon(file) {
            if (!file) return;
            const form = new FormData();
            form.append("file", file);
            try {
                const res = await fetch("/api/assets", { method: "POST", body: form });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || "Upload fehlgeschlagen");
                const b = this.ensureButton(this.selected);
                b.icon_path = payload.path;
                b.preview_url = payload.preview_url;
                this.markDirty();
            } catch (err) {
                this.toast(err.message, "err");
            }
        },

        async importIcon(b, assetId) {
            try {
                const res = await fetch("/api/builtin-assets/import", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ asset_id: assetId }),
                });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || "Icon konnte nicht übernommen werden");
                b.icon_path = payload.path;
                b.preview_url = payload.preview_url;
                this.markDirty();
            } catch (err) {
                this.toast(err.message, "err");
            }
        },

        async openCatalog() {
            this.catalog.open = true;
            this.$nextTick(() => this.$refs.catalogSearch?.focus());
            if (this.catalog.loaded) return;
            try {
                const res = await fetch("/api/builtin-assets");
                this.catalog.items = (await res.json()).items || [];
                this.catalog.loaded = true;
            } catch (_err) {
                this.toast("Katalog konnte nicht geladen werden", "err");
            }
        },

        catalogMatches() {
            const q = this.catalog.query.trim().toLowerCase();
            const terms = q.split(/\s+/).filter(Boolean);
            return this.catalog.items.filter((icon) => {
                if (this.catalog.style !== "all" && icon.style !== this.catalog.style) return false;
                if (!terms.length) return true;
                const hay = `${icon.name} ${icon.search_terms.join(" ")}`.toLowerCase();
                return terms.every((t) => hay.includes(t));
            });
        },

        catalogResults() {
            return this.catalogMatches().slice(0, 240);
        },

        catalogSummary() {
            if (!this.catalog.loaded) return "Lade …";
            const total = this.catalogMatches().length;
            return total > 240 ? `240 von ${total} Treffern – Suche verfeinern` : `${total} Treffer`;
        },

        async useCatalogIcon(icon) {
            await this.importIcon(this.ensureButton(this.selected), icon.asset_id);
            this.catalog.open = false;
        },

        // ───────────── Infofenster ─────────────

        infoMode() {
            const sw = this.editor.small_window;
            if (!sw.show_metrics) return "clock";
            return sw.rotate_every_s ? "rotate" : "stats";
        },

        setInfoMode(mode) {
            const sw = this.editor.small_window;
            sw.show_metrics = mode !== "clock";
            sw.rotate_every_s = mode === "rotate" ? (sw.rotate_every_s || 10) : null;
            this.markDirty();
        },

        infoShowsStats() {
            const sw = this.editor.small_window;
            if (!sw.show_metrics) return false;
            if (!sw.rotate_every_s) return true;
            return Math.floor(Date.now() / 1000 / sw.rotate_every_s) % 2 === 1;
        },

        infoStatLines() {
            if (this.editor.small_window.metrics_items.length) {
                return (this.preview.metrics || []).map((m) => ({ label: m.label, value: m.value }));
            }
            return [
                { label: "CPU", value: `${this.preview.cpu_percent}%` },
                { label: "RAM", value: `${this.preview.mem_percent}%` },
            ];
        },

        toggleMetric(id) {
            const items = this.editor.small_window.metrics_items;
            this.editor.small_window.metrics_items = items.includes(id) ? items.filter((m) => m !== id) : [...items, id];
            this.markDirty();
        },

        toggleSensor(id) {
            const items = this.editor.small_window.temperature_sensors;
            this.editor.small_window.temperature_sensors = items.includes(id) ? items.filter((s) => s !== id) : [...items, id];
            this.markDirty();
        },
    };
};
