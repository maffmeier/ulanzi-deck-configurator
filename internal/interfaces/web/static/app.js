"use strict";

const INFO_INDEX = 13;

// D200: 13 keys on a 5x3 grid, the wide info window takes the last two
// cells of the bottom row.
const SLOTS = Object.freeze([
    ...Array.from({ length: 13 }, (_, i) => ({ index: i, row: Math.floor(i / 5) + 1, column: (i % 5) + 1, span: 1 })),
    { index: INFO_INDEX, row: 3, column: 4, span: 2, kind: "info" },
]);

const ACTION_TYPES = Object.freeze([
    "none", "shell", "shortcut", "type_text", "url", "http", "switch_page",
    "predefined_command", "multi", "timer", "audio", "meeting", "home_assistant", "obs",
]);
const PRESS_MODES = Object.freeze(["tap", "repeat", "hold"]);
const TIMER_OPS = Object.freeze(["toggle", "start", "pause", "reset"]);
const OBS_OPS = Object.freeze([
    "switch_scene", "toggle_record", "start_record", "stop_record", "toggle_stream",
    "start_stream", "stop_stream", "toggle_mute", "toggle_replay_buffer", "save_replay",
]);
const AUDIO_OPS = Object.freeze(["volume_up", "volume_down", "mute", "mic_mute", "output_next", "output_set"]);
const AUDIO_SHOWS = Object.freeze(["volume", "mic", "output"]);
// Mirrors deck.MeetingOps.
const MEETING_OPS = Object.freeze({
    teams: ["mute", "camera", "hand", "blur", "leave"],
    zoom: ["mute", "camera", "hand", "share", "leave"],
    discord: ["mute", "deafen"],
    meet: ["mute", "camera", "hand"],
});
const MEETING_SHOWS = Object.freeze(["mute", "camera", "hand"]);
const WIDGET_TYPES = Object.freeze([
    { id: "clock" }, { id: "metrics" }, { id: "command" }, { id: "graph" },
    { id: "image", infoOnly: true }, { id: "timer" }, { id: "media" }, { id: "audio" }, { id: "meeting" }, { id: "home_assistant" }, { id: "obs" },
]);
const GRAPH_METRICS = Object.freeze(["cpu", "memory", "network"]);
const METRICS = Object.freeze(["cpu", "memory", "gpu", "temperature", "disk", "network", "battery"]);
const FONTS = Object.freeze(["DejaVu Sans", "DejaVu Serif", "DejaVu Sans Mono", "Liberation Sans", "Liberation Serif"]);

const ACTION_ICONS = Object.freeze({
    shell: "fa:solid:terminal",
    shortcut: "fa:solid:keyboard",
    type_text: "fa:solid:i-cursor",
    url: "fa:solid:globe",
    http: "fa:solid:paper-plane",
    switch_page: "fa:solid:layer-group",
    predefined_command: "fa:solid:gear",
    multi: "fa:solid:list-ol",
    home_assistant: "fa:solid:house",
    obs: "fa:solid:video",
    audio: "fa:solid:volume-high",
    meeting: "fa:solid:users",
});

// Presets of the action list; label/hint are i18n keys. The backend's
// predefined commands are appended as further groups.
const BASE_GROUPS = Object.freeze([
    {
        id: "general",
        items: [
            { id: "shell", key: "shell", icon: "fa:solid:terminal", action: { type: "shell" }, focus: true },
            { id: "shortcut", key: "shortcut", icon: "fa:solid:keyboard", action: { type: "shortcut" }, focus: true },
            { id: "type_text", key: "typeText", icon: "fa:solid:i-cursor", action: { type: "type_text" }, focus: true },
            { id: "url", key: "url", icon: "fa:solid:globe", action: { type: "url" }, focus: true },
            { id: "http", key: "http", icon: "fa:solid:paper-plane", action: { type: "http", method: "POST" }, focus: true },
            { id: "multi", key: "multi", icon: "fa:solid:list-ol", action: { type: "multi", steps: [{ type: "shell" }, { type: "delay", ms: 500 }] } },
        ],
    },
    {
        id: "navigation",
        items: [
            { id: "page-next", key: "next", icon: "fa:solid:arrow-right", action: { type: "switch_page", page: "@next" }, title: "→" },
            { id: "page-prev", key: "prev", icon: "fa:solid:arrow-left", action: { type: "switch_page", page: "@prev" }, title: "←" },
            { id: "page-back", key: "back", icon: "fa:solid:rotate-left", action: { type: "switch_page", page: "@back" }, title: "↩" },
        ],
    },
    {
        id: "timer",
        items: [
            { id: "timer-toggle", key: "timerToggle", icon: "fa:solid:stopwatch", action: { type: "timer", op: "toggle", minutes: 25 }, live: { type: "timer" } },
            { id: "timer-reset", key: "timerReset", icon: "fa:solid:rotate-left", action: { type: "timer", op: "reset" }, title: "Reset" },
        ],
    },
    {
        id: "displays",
        items: [
            { id: "live-clock", key: "liveClock", icon: "fa:solid:clock", live: { type: "clock", format: "%H:%M" } },
            { id: "live-timer", key: "liveTimer", icon: "fa:solid:hourglass-half", live: { type: "timer" } },
            { id: "live-cpu", key: "liveCpu", icon: "fa:solid:chart-line", live: { type: "graph", metric: "cpu" } },
            { id: "live-media", key: "liveMedia", icon: "fa:solid:music", live: { type: "media" } },
            { id: "live-command", key: "liveCommand", icon: "fa:solid:terminal", live: { type: "command" } },
        ],
    },
    {
        id: "audio",
        items: [
            { id: "audio-up", key: "audioUp", icon: "fa:solid:volume-high", action: { type: "audio", op: "volume_up", step: 5 }, live: { type: "audio", show: "volume" } },
            { id: "audio-down", key: "audioDown", icon: "fa:solid:volume-low", action: { type: "audio", op: "volume_down", step: 5 }, useIcon: true },
            { id: "audio-mute", key: "audioMute", icon: "fa:solid:volume-xmark", action: { type: "audio", op: "mute" }, useIcon: true },
            { id: "audio-mic", key: "audioMic", icon: "fa:solid:microphone-slash", action: { type: "audio", op: "mic_mute" }, live: { type: "audio", show: "mic" } },
            { id: "audio-output", key: "audioOutput", icon: "fa:solid:headphones", action: { type: "audio", op: "output_next" }, live: { type: "audio", show: "output" } },
            { id: "audio-app", key: "audioApp", icon: "fa:solid:sliders", action: { type: "audio", op: "volume_up", step: 5, target: "spotify" }, live: { type: "audio", show: "volume", target: "spotify" }, focus: true },
        ],
    },
    {
        id: "meeting",
        items: [
            { id: "teams-mute", key: "teamsMute", icon: "fa:solid:microphone", action: { type: "meeting", app: "teams", op: "mute" }, live: { type: "meeting", show: "mute" } },
            { id: "teams-camera", key: "teamsCamera", icon: "fa:solid:video", action: { type: "meeting", app: "teams", op: "camera" }, live: { type: "meeting", show: "camera" } },
            { id: "teams-hand", key: "teamsHand", icon: "fa:solid:hand", action: { type: "meeting", app: "teams", op: "hand" }, live: { type: "meeting", show: "hand" } },
            { id: "teams-leave", key: "teamsLeave", icon: "fa:solid:phone-slash", action: { type: "meeting", app: "teams", op: "leave" }, useIcon: true },
            { id: "zoom-mute", key: "zoomMute", icon: "fa:solid:microphone-slash", action: { type: "meeting", app: "zoom", op: "mute" }, useIcon: true },
            { id: "zoom-camera", key: "zoomCamera", icon: "fa:solid:video-slash", action: { type: "meeting", app: "zoom", op: "camera" }, useIcon: true },
            { id: "discord-mute", key: "discordMute", icon: "fa:brands:discord", action: { type: "meeting", app: "discord", op: "mute" }, useIcon: true },
            { id: "discord-deafen", key: "discordDeafen", icon: "fa:solid:headphones", action: { type: "meeting", app: "discord", op: "deafen" }, useIcon: true },
            { id: "meet-mute", key: "meetMute", icon: "fa:solid:microphone-slash", action: { type: "meeting", app: "meet", op: "mute" }, useIcon: true },
        ],
    },
    {
        id: "smart",
        items: [
            { id: "ha-toggle", key: "haToggle", icon: "fa:solid:lightbulb", action: { type: "home_assistant", service: "light.toggle" }, useIcon: true, focus: true },
            { id: "ha-state", key: "haState", icon: "fa:solid:house", live: { type: "home_assistant" } },
            { id: "obs-scene", key: "obsScene", icon: "fa:solid:film", action: { type: "obs", op: "switch_scene" }, focus: true },
            { id: "obs-record", key: "obsRecord", icon: "fa:solid:circle-dot", action: { type: "obs", op: "toggle_record" }, live: { type: "obs", show: "record" } },
            { id: "obs-stream", key: "obsStream", icon: "fa:solid:tower-broadcast", action: { type: "obs", op: "toggle_stream" }, live: { type: "obs", show: "stream" } },
            { id: "obs-mute", key: "obsMute", icon: "fa:solid:microphone-slash", action: { type: "obs", op: "toggle_mute", target: "Mic/Aux" }, useIcon: true },
        ],
    },
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

function emptyAction(type = "none") {
    return {
        type, cmd: "", keys: "", command_id: "", url: "", page: "", op: "toggle", minutes: 25,
        method: "POST", body: "", headers: {}, text: "", steps: [], ms: 500,
        service: "", entity_id: "", target: "", step: 5, topic: "", retain: false, app: "", plugin: "", settings: {},
    };
}

function normalizeAction(raw) {
    const a = { ...emptyAction(), ...(raw || {}) };
    a.headers = a.headers || {};
    a.settings = a.settings || {};
    a.steps = (a.steps || []).map(normalizeAction);
    return a;
}

function emptyWidget(type = "none") {
    return {
        type, title: "", format: "%H:%M", items: type === "metrics" ? ["cpu", "memory"] : [],
        cmd: "", interval_s: 5, metric: "cpu", paths: [], ok_color: "", fail_color: "",
        entity_id: "", attribute: "", active_state: "on", show: type === "audio" ? "volume" : "scene",
        target: "", topic: "", url: "", plugin: "", settings: {},
    };
}

function normalizeWidget(raw) {
    if (!raw) return emptyWidget();
    return { ...emptyWidget(raw.type), ...raw, items: raw.items || [], paths: raw.paths || [], settings: raw.settings || {} };
}

function normalizeButton(raw) {
    return {
        index: raw.index,
        label: raw.label || "",
        icon_path: raw.icon_path || "",
        preview_url: raw.preview_url || "",
        action: normalizeAction(raw.action),
        text_style: { ...DEFAULT_STYLE, ...(raw.text_style || {}) },
        press: raw.press || "tap",
        long_press: normalizeAction(raw.long_press),
        live: normalizeWidget(raw.live),
        toggle: raw.toggle ? {
            label: raw.toggle.label || "", icon_path: raw.toggle.icon_path || "",
            preview_url: raw.toggle.preview_url || "", action: normalizeAction(raw.toggle.action),
        } : null,
    };
}

function isEmptyButton(b) {
    return !b.label && !b.icon_path && (!b.action || b.action.type === "none") &&
        (!b.long_press || b.long_press.type === "none") && (!b.live || b.live.type === "none") && !b.toggle &&
        JSON.stringify({ ...DEFAULT_STYLE, ...b.text_style }).toLowerCase() === JSON.stringify(DEFAULT_STYLE).toLowerCase();
}

function iconUrl(assetId) {
    return assetId ? `/api/builtin-asset?asset_id=${encodeURIComponent(assetId)}` : "";
}

function storedLang() {
    try {
        const v = localStorage.getItem("ulanzi-deck-lang");
        if (v === "de" || v === "en") return v;
    } catch (_err) { /* storage may be unavailable */ }
    return (navigator.language || "en").toLowerCase().startsWith("de") ? "de" : "en";
}

window.deckApp = function deckApp() {
    return {
        INFO_INDEX, FONTS, METRICS, PRESS_MODES, TIMER_OPS, OBS_OPS, GRAPH_METRICS, AUDIO_OPS, AUDIO_SHOWS, MEETING_OPS, MEETING_SHOWS,
        lang: storedLang(),
        previews: {},
        widgetFields: "",
        actionFields: "",
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
        recordingTarget: null,
        previewToggled: false,
        imageTarget: "main",
        toasts: [],
        catalog: { open: false, loaded: false, items: [], query: "", style: "all", target: "main" },
        pageDialog: { open: false, original: "", name: "", folder: false, back_index: 0, apps: "" },
        recentApps: [],
        settings: { open: false, tab: "general" },
        secrets: {},
        secretsTest: {},
        // Undo history as JSON snapshots of the editor state.
        history: { past: [], future: [], last: "", saved: "" },
        historyTimer: null,
        versions: { open: false, items: [] },
        importDialog: { open: false, name: "", data: null },

        get shellPlaceholder() {
            if (navigator.platform.startsWith("Win")) return "start notepad";
            return navigator.platform.startsWith("Mac") ? "open -a Terminal" : "swaymsg exec alacritty";
        },

        async init() {
            this.actionFields = document.getElementById("tpl-action-fields").innerHTML;
            this.widgetFields = document.getElementById("tpl-widget-fields").innerHTML;
            document.documentElement.lang = this.lang;
            await Promise.all([this.load(), this.loadPredefined()]);
            this.pollHealth();
            this.pollWidgetPreviews();
            this.pollPreview();
            this.loadSensors();
            window.addEventListener("beforeunload", (e) => {
                if (this.dirty) e.preventDefault();
            });
        },

        // ───────────── i18n ─────────────

        t(key, params = {}) {
            const text = window.I18N[this.lang]?.[key] ?? window.I18N.de[key] ?? key;
            return text.replace(/\{(\w+)\}/g, (_, name) => params[name] ?? "");
        },

        setLang(lang) {
            this.lang = lang;
            document.documentElement.lang = lang;
            try {
                localStorage.setItem("ulanzi-deck-lang", lang);
            } catch (_err) { /* storage may be unavailable */ }
        },

        // ───────────── Load & save ─────────────

        async load() {
            this.busy = true;
            try {
                const res = await fetch("/api/editor");
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || this.t("msg.loadFailed"));
                this.editor = this.normalizeEditor(payload);
                if (!this.editor.pages.some((p) => p.name === this.currentPage)) {
                    this.currentPage = this.editor.default_page;
                }
                this.selected = null;
                this.resetHistory();
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
                language: payload.language || "",
                pages: (payload.pages || []).map((p) => ({
                    name: p.name, folder: Boolean(p.folder), back_index: p.back_index || 0, apps: p.apps || [],
                    buttons: (p.buttons || []).map(normalizeButton),
                })),
                display: {
                    dim_after_s: 0, dim_brightness: 10, off_when_locked: false, night_enabled: false,
                    night_from: "22:00", night_to: "07:00", night_brightness: 20, press_feedback: false, ...(payload.display || {}),
                },
                fixed_buttons: (payload.fixed_buttons || []).map(normalizeButton),
                small_window: {
                    enabled: Boolean(sw.enabled),
                    interval_s: sw.interval_s || 2,
                    time_format: sw.time_format || "%H:%M",
                    show_metrics: sw.show_metrics !== false,
                    rotate_every_s: sw.rotate_every_s ?? null,
                    background_color: sw.background_color || "#000000",
                    metrics_items: sw.metrics_items || [],
                    widgets: (sw.widgets || []).map(normalizeWidget),
                    temperature_sensors: sw.temperature_sensors || [],
                    temperature_separator: sw.temperature_separator === "|" ? "|" : " ",
                },
            };
        },

        async reload() {
            if (this.dirty && !confirm(this.t("msg.discardConfirm"))) return;
            await this.load();
            this.toast(this.t("msg.discarded"));
        },

        serializeAction(a) {
            if (!a || a.type === "none") return null;
            return {
                ...a,
                minutes: Number(a.minutes) || 0,
                ms: Number(a.ms) || 0,
                steps: a.type === "multi" ? a.steps.map((s) => this.serializeAction(s)).filter(Boolean) : [],
            };
        },

        serializeButtons(buttons) {
            return buttons.filter((b) => !isEmptyButton(b)).map((b) => ({
                index: b.index,
                label: b.label,
                icon_path: b.icon_path || null,
                action: this.serializeAction(b.action) || { type: "none" },
                text_style: { ...b.text_style, font_size: Number(b.text_style.font_size) },
                press: b.press || "tap",
                long_press: b.press === "tap" ? this.serializeAction(b.long_press) : null,
                live: b.live && b.live.type !== "none" ? { ...b.live, interval_s: Number(b.live.interval_s) } : null,
                toggle: b.toggle ? { label: b.toggle.label, icon_path: b.toggle.icon_path || null, action: this.serializeAction(b.toggle.action) } : null,
            }));
        },

        payload() {
            const sw = this.editor.small_window;
            return {
                default_page: this.editor.default_page,
                brightness: Number(this.editor.brightness),
                language: this.editor.language,
                pages: this.editor.pages.map((p) => ({
                    name: p.name, folder: p.folder, back_index: Number(p.back_index) || 0, apps: p.apps || [],
                    buttons: this.serializeButtons(p.buttons),
                })),
                display: {
                    ...this.editor.display,
                    dim_after_s: Number(this.editor.display.dim_after_s) || 0,
                    dim_brightness: Number(this.editor.display.dim_brightness) || 0,
                    night_brightness: Number(this.editor.display.night_brightness) || 0,
                },
                fixed_buttons: this.serializeButtons(this.editor.fixed_buttons),
                small_window: {
                    ...sw,
                    widgets: sw.widgets.map((w) => ({ ...w, interval_s: Number(w.interval_s) })),
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
                if (!res.ok) throw new Error(payload.error || payload.detail || this.t("msg.saveFailed"));
                const selected = this.selected;
                this.editor = this.normalizeEditor(payload);
                this.history.last = this.history.saved = JSON.stringify(this.editor);
                this.dirty = false;
                this.selected = selected;
                this.toast(this.t(this.health.deck_connected ? "msg.savedLive" : "msg.saved"), "ok");
            } catch (err) {
                this.toast(err.message, "err", 8000);
            } finally {
                this.busy = false;
            }
        },

        markDirty() {
            this.dirty = true;
            clearTimeout(this.historyTimer);
            this.historyTimer = setTimeout(() => this.recordHistory(), 400);
        },

        // ───────────── Undo & versions ─────────────

        resetHistory() {
            clearTimeout(this.historyTimer);
            const state = JSON.stringify(this.editor);
            this.history = { past: [], future: [], last: state, saved: state };
            this.dirty = false;
        },

        recordHistory() {
            clearTimeout(this.historyTimer);
            const state = JSON.stringify(this.editor);
            if (state === this.history.last) return;
            this.history.past = [...this.history.past, this.history.last].slice(-100);
            this.history.future = [];
            this.history.last = state;
            this.dirty = state !== this.history.saved;
        },

        restoreState(state) {
            this.history.last = state;
            this.editor = JSON.parse(state);
            if (!this.editor.pages.some((p) => p.name === this.currentPage)) this.currentPage = this.editor.default_page;
            this.dirty = state !== this.history.saved;
        },

        undo() {
            this.recordHistory();
            const past = [...this.history.past];
            if (!past.length) return;
            this.history.future = [...this.history.future, this.history.last];
            this.restoreState(past.pop());
            this.history.past = past;
        },

        redo() {
            const future = [...this.history.future];
            if (!future.length) return;
            this.history.past = [...this.history.past, this.history.last];
            this.restoreState(future.pop());
            this.history.future = future;
        },

        async openVersions() {
            this.versions = { open: true, items: [] };
            try {
                const res = await fetch("/api/backups");
                this.versions.items = (await res.json()).items || [];
            } catch (_err) {
                this.toast(this.t("versions.failed"), "err");
            }
        },

        versionTime(item) {
            return new Date(item.saved).toLocaleString(this.lang === "de" ? "de-DE" : "en-GB");
        },

        async restoreVersion(item) {
            if (this.dirty && !confirm(this.t("msg.discardConfirm"))) return;
            try {
                const res = await fetch(`/api/backups/${encodeURIComponent(item.name)}/restore`, { method: "POST" });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.error || payload.detail || this.t("msg.saveFailed"));
                this.editor = this.normalizeEditor(payload);
                if (!this.editor.pages.some((p) => p.name === this.currentPage)) this.currentPage = this.editor.default_page;
                this.selected = null;
                this.resetHistory();
                this.versions.open = false;
                this.toast(this.t("versions.restored", { time: this.versionTime(item) }), "ok");
            } catch (err) {
                this.toast(err.message, "err", 8000);
            }
        },

        // ───────────── Actions ─────────────

        actionTypes() {
            return ACTION_TYPES;
        },

        stepTypes() {
            return [...ACTION_TYPES.filter((t) => t !== "none" && t !== "multi"), "delay"];
        },

        // Selects show their first option when the bound value is empty, so
        // give every action a real default instead of a misleading blank.
        actionTypeChanged(a) {
            if (a.type === "switch_page" && !a.page) a.page = "@next";
            if (a.type === "predefined_command" && !a.command_id) a.command_id = this.predefined[0]?.id || "";
            if (a.type === "timer" && !TIMER_OPS.includes(a.op)) a.op = "toggle";
            if (a.type === "obs" && !OBS_OPS.includes(a.op)) a.op = "toggle_record";
            if (a.type === "audio" && !AUDIO_OPS.includes(a.op)) a.op = "volume_up";
            if (a.type === "meeting") this.meetingAppChanged(a);
            if (a.type === "http" && !a.method) a.method = "POST";
            if (a.type === "multi" && !a.steps.length) a.steps = [emptyAction("shell")];
            if (this.cur && a === this.cur.action && !this.pressModeAllowed(this.cur.press)) this.cur.press = "tap";
            this.markDirty();
        },

        // Mirrors deck.Button.ValidateBehavior.
        pressModeAllowed(mode) {
            const type = this.cur?.action.type;
            if (this.cur?.toggle && mode !== "tap") return false;
            if (mode === "repeat") return type !== "none" && type !== "switch_page";
            if (mode === "hold") return type === "shortcut" || type === "predefined_command";
            return true;
        },

        meetingAppChanged(a) {
            if (!MEETING_OPS[a.app]) a.app = "teams";
            if (!MEETING_OPS[a.app].includes(a.op)) a.op = "mute";
            this.markDirty();
        },

        headersText(a) {
            return Object.entries(a.headers || {}).map(([k, v]) => `${k}: ${v}`).join("\n");
        },

        setHeaders(a, text) {
            const headers = {};
            for (const line of text.split("\n")) {
                const i = line.indexOf(":");
                if (i > 0) headers[line.slice(0, i).trim()] = line.slice(i + 1).trim();
            }
            a.headers = headers;
            this.markDirty();
        },

        addStep(a, type) {
            a.steps.push(type === "delay" ? { ...emptyAction("delay"), ms: 500 } : emptyAction(type));
            this.markDirty();
        },

        moveStep(a, i, delta) {
            const j = i + delta;
            if (j < 0 || j >= a.steps.length) return;
            [a.steps[i], a.steps[j]] = [a.steps[j], a.steps[i]];
            this.markDirty();
        },

        removeStep(a, i) {
            a.steps.splice(i, 1);
            this.markDirty();
        },

        setToggle(on) {
            this.cur.toggle = on ? { label: "", icon_path: "", preview_url: "", action: emptyAction() } : null;
            if (on) this.cur.press = "tap";
            this.markDirty();
        },

        pressBadge(b) {
            if (!b) return "";
            if (b.toggle) return "⇄";
            if (b.press === "repeat") return "⟳";
            if (b.press === "hold") return "⇩";
            if (b.long_press && b.long_press.type !== "none") return "⧗";
            return "";
        },

        describeAction(a) {
            switch (a.type) {
                case "shell": return `${this.t("action.shell")}: ${a.cmd}`;
                case "shortcut": return `${this.t("action.shortcut")}: ${a.keys}`;
                case "type_text": return `${this.t("action.type_text")}: ${a.text.slice(0, 30)}`;
                case "url": return a.url;
                case "http": return `${a.method} ${a.url}`;
                case "switch_page":
                    if (a.page === "@next") return this.t("nav.next");
                    if (a.page === "@prev") return this.t("nav.prev");
                    if (a.page === "@back") return this.t("nav.back");
                    return this.t("nav.page", { name: a.page });
                case "predefined_command": return this.predefinedLabel(a.command_id);
                case "multi": return `${this.t("action.multi")} (${a.steps.length})`;
                case "timer": return `${this.t("action.timer")}: ${this.t("timer." + a.op)}`;
                case "home_assistant": return `${a.service} ${a.entity_id}`;
                case "obs": return `OBS: ${this.t("obs." + a.op)} ${a.target}`;
                case "audio": return `${this.t("audio." + a.op)} ${a.target}`.trim();
                case "meeting": return `${this.t("meetingApp." + a.app)}: ${this.t("meeting." + a.op)}`;
            }
            return "";
        },

        actionIconUrl(action) {
            if (!action) return "";
            if (action.type === "predefined_command") {
                return iconUrl(this.predefined.find((p) => p.id === action.command_id)?.icon || ACTION_ICONS.predefined_command);
            }
            if (action.type === "timer") return iconUrl(action.op === "reset" ? "fa:solid:rotate-left" : "fa:solid:stopwatch");
            if (action.type === "switch_page") {
                const icons = { "@prev": "fa:solid:arrow-left", "@next": "fa:solid:arrow-right", "@back": "fa:solid:rotate-left" };
                return iconUrl(icons[action.page] || "fa:solid:layer-group");
            }
            return iconUrl(ACTION_ICONS[action.type]);
        },

        iconUrl,

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
                    if (res.ok) this.preview = await res.json();
                } catch (_err) { /* preview is best effort */ }
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

        predefinedLabel(id) {
            const p = this.predefined.find((x) => x.id === id);
            if (this.lang === "en") return window.I18N_PREDEFINED_EN.labels[id] || p?.label || id;
            return p?.label || id;
        },

        predefinedGroups() {
            const groups = [];
            for (const p of this.predefined) {
                let g = groups.find((x) => x.id === p.group);
                if (!g) {
                    const name = this.lang === "en" ? (window.I18N_PREDEFINED_EN.groups[p.group] || p.group) : p.group;
                    groups.push(g = { id: p.group, name, items: [] });
                }
                g.items.push({ ...p, label: this.predefinedLabel(p.id) });
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
            setTimeout(() => { this.toasts = this.toasts.filter((x) => x.id !== id); }, ms);
        },

        // ───────────── Export & import ─────────────

        // Export works on the saved file, so unsaved edits are saved first.
        async exportProfile(page = "") {
            if (this.dirty) {
                if (!confirm(this.t("profile.saveFirst"))) return;
                await this.save();
                if (this.dirty) return;
            }
            const link = document.createElement("a");
            link.href = page ? `/api/export?page=${encodeURIComponent(page)}` : "/api/export";
            document.body.appendChild(link);
            link.click();
            link.remove();
        },

        async importProfile(file) {
            if (!file) return;
            const form = new FormData();
            form.append("file", file);
            try {
                const res = await fetch("/api/import", { method: "POST", body: form });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || this.t("profile.importFailed"));
                this.importDialog = { open: true, name: file.name, data: this.normalizeEditor(payload) };
                this.settings.open = false;
            } catch (err) {
                this.toast(err.message, "err", 8000);
            }
        },

        // Imported pages get free names; keys on pinned positions are dropped
        // because pinned keys win on every page.
        importPages() {
            const data = this.importDialog.data;
            const renamed = {};
            let dropped = 0;
            for (const p of data.pages) {
                renamed[p.name] = this.uniquePageName(p.name);
                const buttons = p.buttons.filter((b) => !this.isFixed(b.index));
                dropped += p.buttons.length - buttons.length;
                this.editor.pages.push({ ...p, name: renamed[p.name], buttons });
            }
            const retarget = (a) => {
                if (!a) return;
                if (a.type === "switch_page" && renamed[a.page]) a.page = renamed[a.page];
                (a.steps || []).forEach(retarget);
            };
            for (const p of data.pages) {
                const page = this.editor.pages.find((x) => x.name === renamed[p.name]);
                page.buttons.forEach((b) => { retarget(b.action); retarget(b.long_press); retarget(b.toggle?.action); });
            }
            this.currentPage = renamed[data.pages[0].name];
            this.selected = null;
            this.importDialog.open = false;
            this.markDirty();
            this.toast(this.t(dropped ? "profile.importedDropped" : "profile.imported", { n: data.pages.length, dropped }), "ok", 6000);
        },

        importReplace() {
            this.editor = { ...this.importDialog.data, path: this.editor.path };
            this.currentPage = this.editor.default_page;
            this.selected = null;
            this.importDialog.open = false;
            this.markDirty();
            this.toast(this.t("profile.replaced"), "ok");
        },

        // ───────────── Pages ─────────────

        page() {
            return this.editor.pages.find((p) => p.name === this.currentPage);
        },

        pageNames() {
            return this.editor ? this.editor.pages.map((p) => p.name) : [];
        },

        pageTabTitle(page) {
            const parts = [];
            if (page.name === this.editor.default_page) parts.push(this.t("page.tabDefault"));
            if (page.folder) parts.push(this.t("page.tabFolder"));
            if (page.apps.length) parts.push(this.t("page.tabApps", { apps: page.apps.join(", ") }));
            parts.push(this.t("page.tabHint"));
            return parts.join(" · ");
        },

        selectPage(name) {
            this.currentPage = name;
            if (this.selected !== null && !this.isFixed(this.selected)) this.selected = null;
        },

        uniquePageName(base) {
            let name = base;
            for (let i = 2; this.pageNames().includes(name); i++) name = `${base} ${i}`;
            return name;
        },

        addPage() {
            const name = this.uniquePageName(this.t("page.newName", { n: this.editor.pages.length + 1 }));
            this.editor.pages.push({ name, folder: false, back_index: 0, apps: [], buttons: [] });
            this.currentPage = name;
            this.selected = null;
            this.markDirty();
        },

        async openPageSettings(name) {
            const p = this.editor.pages.find((x) => x.name === name);
            this.pageDialog = { open: true, original: name, name, folder: p.folder, back_index: p.back_index, apps: p.apps.join("\n") };
            try {
                const res = await fetch("/api/desktop/recent-apps");
                this.recentApps = res.ok ? (await res.json()).items || [] : [];
            } catch (_err) {
                this.recentApps = [];
            }
        },

        addAppToDialog(app) {
            const apps = this.pageDialog.apps.split("\n").map((x) => x.trim()).filter(Boolean);
            if (!apps.includes(app)) apps.push(app);
            this.pageDialog.apps = apps.join("\n");
        },

        applyPageSettings() {
            const d = this.pageDialog;
            const page = this.editor.pages.find((x) => x.name === d.original);
            const name = d.name.trim();
            if (!name) return;
            if (name !== d.original) {
                if (this.pageNames().includes(name)) {
                    this.toast(this.t("page.exists", { name }), "err");
                    return;
                }
                this.renamePage(d.original, name);
            }
            page.folder = d.folder;
            page.back_index = Number(d.back_index) || 0;
            page.apps = d.apps.split("\n").map((x) => x.trim()).filter(Boolean);
            this.pageDialog.open = false;
            this.markDirty();
        },

        renamePage(oldName, name) {
            this.editor.pages.find((p) => p.name === oldName).name = name;
            const retarget = (a) => {
                if (!a) return;
                if (a.type === "switch_page" && a.page === oldName) a.page = name;
                (a.steps || []).forEach(retarget);
            };
            const all = [...this.editor.pages.flatMap((p) => p.buttons), ...this.editor.fixed_buttons];
            all.forEach((b) => { retarget(b.action); retarget(b.long_press); retarget(b.toggle?.action); });
            if (this.editor.default_page === oldName) this.editor.default_page = name;
            if (this.currentPage === oldName) this.currentPage = name;
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
            if (this.page().buttons.some((b) => !isEmptyButton(b)) && !confirm(this.t("page.confirmDelete", { name }))) return;
            this.editor.pages = this.editor.pages.filter((p) => p.name !== name);
            if (this.editor.default_page === name) this.editor.default_page = this.editor.pages[0].name;
            this.currentPage = this.editor.pages[0].name;
            this.selected = null;
            this.markDirty();
        },

        // ───────────── Keys ─────────────

        isFixed(index) {
            return this.editor.fixed_buttons.some((b) => b.index === index);
        },

        button(index) {
            if (!this.editor) return null;
            return this.editor.fixed_buttons.find((b) => b.index === index)
                || this.page()?.buttons.find((b) => b.index === index)
                || null;
        },

        // The key as the device shows it, honouring the toggle preview.
        shown(index) {
            const b = this.button(index);
            if (!b || !b.toggle || !this.previewToggled) return b;
            return { ...b, label: b.toggle.label, icon_path: b.toggle.icon_path, preview_url: b.toggle.preview_url };
        },

        hasToggles() {
            return this.editor && [...(this.page()?.buttons || []), ...this.editor.fixed_buttons].some((b) => b.toggle);
        },

        ensureButton(index) {
            let b = this.button(index);
            if (!b) {
                this.page().buttons.push(normalizeButton({ index }));
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
            const b = this.shown(slot.index);
            return {
                info: slot.kind === "info",
                selected: this.selected === slot.index,
                empty: slot.kind !== "info" && (!b || (!b.preview_url && !b.label && !this.hasLive(b))),
                "drop-target": this.dragOverKey === slot.index,
            };
        },

        keyTooltip(slot) {
            const b = this.button(slot.index);
            const name = slot.kind === "info" ? this.t("info.name") : this.t("key.title", { n: slot.index + 1 });
            if (!b) return name;
            const parts = [name];
            if (b.action.type !== "none") parts.push(this.describeAction(b.action));
            if (b.toggle) parts.push(this.t("key.tooltipToggle"));
            if (b.press === "repeat") parts.push(this.t("key.tooltipRepeat"));
            if (b.press === "hold") parts.push(this.t("key.tooltipHold"));
            if (b.long_press && b.long_press.type !== "none") parts.push(this.t("key.tooltipLong", { action: this.describeAction(b.long_press) }));
            return parts.join(" · ");
        },

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
                if (others.length && !confirm(this.t("msg.pinConflict", { n: index + 1, pages: others.map((p) => `„${p.name}“`).join(", ") }))) return;
                this.editor.pages.forEach((p) => { p.buttons = p.buttons.filter((x) => x.index !== index); });
                this.editor.fixed_buttons.push(b);
            }
            this.markDirty();
        },

        onKey(event) {
            if (this.recordingTarget) {
                this.captureShortcut(event);
                return;
            }
            const typing = ["INPUT", "SELECT", "TEXTAREA"].includes(event.target.tagName);
            const mod = event.ctrlKey || event.metaKey;
            if (mod && event.key === "s") {
                event.preventDefault();
                if (this.dirty) this.save();
            } else if (!typing && mod && (event.key === "y" || (event.key.toLowerCase() === "z" && event.shiftKey))) {
                event.preventDefault();
                this.redo();
            } else if (!typing && mod && event.key === "z") {
                event.preventDefault();
                this.undo();
            } else if (!typing && (event.key === "Delete" || event.key === "Backspace") && this.selected !== null) {
                event.preventDefault();
                this.clearSelected();
            } else if (!typing && event.key === "Escape") {
                this.selected = null;
            }
        },

        recordShortcut(a) {
            this.recordingTarget = this.recordingTarget === a ? null : a;
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
            this.recordingTarget.keys = [...mods, key].join("+");
            this.recordingTarget = null;
            this.markDirty();
        },

        // ───────────── Action list & drag and drop ─────────────

        presetFor(item) {
            return {
                ...item,
                label: this.t(`preset.${item.key}`),
                hint: this.t(`preset.${item.key}Hint`),
            };
        },

        filteredActionGroups() {
            const q = this.actionQuery.trim().toLowerCase();
            const pageItems = this.pageNames().map((name) => ({
                id: `page:${name}`, label: this.t("nav.toPage", { name }), icon: "fa:solid:layer-group",
                hint: this.t("nav.toPageHint", { name }), action: { type: "switch_page", page: name }, title: name,
            }));
            const groups = [
                ...BASE_GROUPS.map((g) => {
                    const items = g.items.map((i) => this.presetFor(i));
                    return { id: g.id, name: this.t(`group.${g.id}`), items: g.id === "navigation" ? [...items, ...pageItems] : items };
                }),
                ...this.predefinedGroups().map((g) => ({
                    id: g.id,
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
                    this.toast(this.t("msg.infoNoImage"), "err");
                    return;
                }
                this.selected = index;
                this.imageTarget = "main";
                await this.uploadIcon(dt.files[0]);
            }
        },

        dropOnPage(event, pageName) {
            this.dragOverPage = null;
            const keyIndex = event.dataTransfer.getData("application/x-deck-key");
            if (keyIndex === "" || pageName === this.currentPage) return;
            const index = Number(keyIndex);
            if (this.isFixed(index)) {
                this.toast(this.t("msg.alreadyFixed"), "err");
                return;
            }
            const source = this.page().buttons.find((b) => b.index === index);
            if (!source) return;
            const target = this.editor.pages.find((p) => p.name === pageName);
            target.buttons = target.buttons.filter((b) => b.index !== index);
            target.buttons.push(JSON.parse(JSON.stringify(source)));
            this.toast(this.t("msg.copiedTo", { n: index + 1, name: pageName }), "ok");
            this.markDirty();
        },

        moveKey(from, to) {
            if (from === to) return;
            if (from === INFO_INDEX || to === INFO_INDEX) {
                this.toast(this.t("msg.infoNoMove"), "err");
                return;
            }
            const fromFixed = this.isFixed(from);
            if (fromFixed !== this.isFixed(to) && this.button(to)) {
                this.toast(this.t("msg.mixedMove"), "err");
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
                this.toast(this.t("msg.selectKeyFirst"), "err");
                return;
            }
            this.applyPreset(this.selected, preset);
        },

        async applyPreset(index, preset) {
            const b = this.ensureButton(index);
            if (preset.live && !preset.action) {
                if (index === INFO_INDEX) {
                    this.editor.small_window.widgets.push(normalizeWidget(preset.live));
                    this.editor.small_window.enabled = true;
                } else {
                    b.live = normalizeWidget(preset.live);
                }
                this.selected = index;
                this.markDirty();
                return;
            }
            if (preset.live && index !== INFO_INDEX && (!b.live || b.live.type === "none")) {
                b.live = normalizeWidget(preset.live);
            }
            b.action = normalizeAction(JSON.parse(JSON.stringify(preset.action)));
            const type = b.action.type;
            if ((b.press === "hold" && type !== "shortcut" && type !== "predefined_command") ||
                (b.press === "repeat" && type === "switch_page")) {
                b.press = "tap";
            }
            if (index !== INFO_INDEX && !b.icon_path && !b.label && !this.hasLive(b)) {
                if (preset.useIcon) {
                    await this.importIcon(b, preset.icon);
                } else if (preset.title) {
                    b.label = preset.title;
                }
            }
            this.selected = index;
            this.markDirty();
            if (preset.focus) {
                this.$nextTick(() => document.querySelector(".props .action-fields input, .props .action-fields textarea")?.focus());
            }
        },

        // ───────────── Images ─────────────

        imageOwner() {
            return this.imageTarget === "toggle" ? this.cur.toggle : this.cur;
        },

        pickImage(target) {
            this.imageTarget = target;
            this.$refs.upload.click();
        },

        async uploadIcon(file) {
            if (!file) return;
            const form = new FormData();
            form.append("file", file);
            try {
                const res = await fetch("/api/assets", { method: "POST", body: form });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || this.t("msg.uploadFailed"));
                const owner = this.imageOwner();
                owner.icon_path = payload.path;
                owner.preview_url = payload.preview_url;
                this.markDirty();
            } catch (err) {
                this.toast(err.message, "err");
            }
        },

        async importIcon(owner, assetId) {
            try {
                const res = await fetch("/api/builtin-assets/import", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ asset_id: assetId }),
                });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || this.t("msg.iconFailed"));
                owner.icon_path = payload.path;
                owner.preview_url = payload.preview_url;
                this.markDirty();
            } catch (err) {
                this.toast(err.message, "err");
            }
        },

        async openCatalog(target) {
            this.imageTarget = target;
            this.catalog.open = true;
            this.$nextTick(() => this.$refs.catalogSearch?.focus());
            if (this.catalog.loaded) return;
            try {
                const res = await fetch("/api/builtin-assets");
                this.catalog.items = (await res.json()).items || [];
                this.catalog.loaded = true;
            } catch (_err) {
                this.toast(this.t("catalog.failed"), "err");
            }
        },

        catalogMatches() {
            const terms = this.catalog.query.trim().toLowerCase().split(/\s+/).filter(Boolean);
            return this.catalog.items.filter((icon) => {
                if (this.catalog.style !== "all" && icon.style !== this.catalog.style) return false;
                if (!terms.length) return true;
                const hay = `${icon.name} ${icon.search_terms.join(" ")}`.toLowerCase();
                return terms.every((term) => hay.includes(term));
            });
        },

        catalogResults() {
            return this.catalogMatches().slice(0, 240);
        },

        catalogSummary() {
            if (!this.catalog.loaded) return this.t("catalog.loading");
            const total = this.catalogMatches().length;
            return this.t(total > 240 ? "catalog.limited" : "catalog.count", { total });
        },

        async useCatalogIcon(icon) {
            await this.importIcon(this.imageOwner(), icon.asset_id);
            this.catalog.open = false;
        },

        // ───────────── Widgets ─────────────

        widgetTypes(onKey) {
            return WIDGET_TYPES.filter((w) => !(onKey && w.infoOnly)).map((w) => w.id);
        },

        addWidget(type) {
            if (!type) return;
            this.editor.small_window.widgets.push(emptyWidget(type));
            this.editor.small_window.enabled = true;
            this.markDirty();
        },

        moveWidget(i, delta) {
            const list = this.editor.small_window.widgets;
            const j = i + delta;
            if (j < 0 || j >= list.length) return;
            [list[i], list[j]] = [list[j], list[i]];
            this.markDirty();
        },

        removeWidget(i) {
            this.editor.small_window.widgets.splice(i, 1);
            this.markDirty();
        },

        setLive(type) {
            this.cur.live = emptyWidget(type);
            this.markDirty();
        },

        toggleWidgetItem(w, id) {
            w.items = w.items.includes(id) ? w.items.filter((x) => x !== id) : [...w.items, id].slice(0, 3);
            this.markDirty();
        },

        setWidgetPaths(w, text) {
            w.paths = text.split("\n").map((p) => p.trim()).filter(Boolean);
            this.markDirty();
        },

        async uploadWidgetImage(w, file) {
            if (!file) return;
            const form = new FormData();
            form.append("file", file);
            try {
                const res = await fetch("/api/assets", { method: "POST", body: form });
                const payload = await res.json();
                if (!res.ok) throw new Error(payload.detail || this.t("msg.uploadFailed"));
                w.paths = [...w.paths, payload.path];
                this.markDirty();
            } catch (err) {
                this.toast(err.message, "err");
            }
        },

        // Mirrors the daemon's rotation so the preview shows the same widget.
        activeInfoWidget() {
            const list = this.editor?.small_window.widgets || [];
            if (!list.length) return null;
            const rotate = Number(this.editor.small_window.rotate_every_s) || 10;
            return list.length === 1 ? list[0] : list[Math.floor(Date.now() / 1000 / rotate) % list.length];
        },

        // Incomplete widgets would only produce validation errors.
        renderable(w) {
            if (w.type === "command") return w.cmd.trim() !== "";
            if (w.type === "image") return w.paths.length > 0;
            if (w.type === "metrics") return w.items.length > 0;
            if (w.type === "home_assistant") return w.entity_id.trim() !== "";
            return w.type !== "none";
        },

        async renderPreview(key, widget, target, background) {
            if (!this.renderable(widget)) return;
            try {
                const res = await fetch("/api/render/widget", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ widget: { ...widget, interval_s: Number(widget.interval_s) || 5 }, target, background }),
                });
                if (!res.ok) return;
                const url = URL.createObjectURL(await res.blob());
                const old = this.previews[key];
                this.previews = { ...this.previews, [key]: url };
                if (old) URL.revokeObjectURL(old);
            } catch (_err) { /* preview is best effort */ }
        },

        async pollWidgetPreviews() {
            if (this.editor) {
                const jobs = [];
                const info = this.activeInfoWidget();
                if (info && this.editor.small_window.enabled) {
                    jobs.push(this.renderPreview("info", info, "info", this.editor.small_window.background_color));
                }
                this.editor.small_window.widgets.forEach((w, i) =>
                    jobs.push(this.renderPreview(`widget-${i}`, w, "info", this.editor.small_window.background_color)));
                for (const slot of SLOTS) {
                    const b = slot.index === INFO_INDEX ? null : this.button(slot.index);
                    if (this.hasLive(b)) jobs.push(this.renderPreview(`key-${slot.index}`, b.live, "key", b.text_style.background_color));
                }
                await Promise.all(jobs);
            }
            setTimeout(() => this.pollWidgetPreviews(), 2000);
        },

        hasLive(b) {
            return Boolean(b?.live && b.live.type !== "none");
        },

        // ───────────── Info window ─────────────

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

        // ───────────── Settings & integrations ─────────────

        openSettings(tab) {
            this.settings = { open: true, tab };
            if (tab === "integrations") this.loadSecrets();
        },

        async loadSecrets() {
            this.secretsTest = {};
            try {
                const res = await fetch("/api/secrets");
                this.secrets = { ...(await res.json()), home_assistant_token: "", obs_password: "", mqtt_password: "" };
            } catch (_err) {
                this.secrets = {};
            }
        },

        async saveSecrets() {
            const res = await fetch("/api/secrets", {
                method: "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(this.secrets),
            });
            if (res.ok) {
                this.secrets = { ...(await res.json()), home_assistant_token: "", obs_password: "", mqtt_password: "" };
                this.toast(this.t("integrations.saved"), "ok");
            } else {
                this.toast((await res.json()).detail || this.t("msg.saveFailed"), "err");
            }
        },

        async testSecrets() {
            this.secretsTest = {};
            const res = await fetch("/api/secrets/test", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(this.secrets),
            });
            this.secretsTest = res.ok ? await res.json() : {};
        },
    };
};
