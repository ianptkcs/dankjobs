.pragma library

// Keys are the raw English strings the tjobs IPC returns or the widget
// hardcodes; pt is the base language, matching Tabela Jobs' own convention.
// tr() falls back to the key itself for any language with no entry.
var strings = {
    "próximo job": { pt: "próximo job", en: "next job", es: "próximo trabajo", fr: "prochain job", de: "nächster Job", it: "prossimo job" },
    "sem jobs pendentes": { pt: "sem jobs pendentes", en: "no pending jobs", es: "sin trabajos pendientes", fr: "aucun job en attente", de: "keine anstehenden Jobs", it: "nessun job in sospeso" },
    "jobs pendentes": { pt: "jobs pendentes", en: "pending jobs", es: "trabajos pendientes", fr: "jobs en attente", de: "anstehende Jobs", it: "job in sospeso" },
    "nenhum job pendente": { pt: "nenhum job pendente", en: "no pending jobs", es: "ningún trabajo pendiente", fr: "aucun job en attente", de: "kein anstehender Job", it: "nessun job in sospeso" },
    "atualizar": { pt: "atualizar", en: "refresh", es: "actualizar", fr: "actualiser", de: "aktualisieren", it: "aggiorna" },
    "active": { pt: "ativo", en: "active", es: "activo", fr: "actif", de: "aktiv", it: "attivo" },
    "paused": { pt: "pausado", en: "paused", es: "pausado", fr: "en pause", de: "pausiert", it: "in pausa" },
};

function tr(key, lang) {
    var entry = strings[key];
    if (entry) {
        if (entry[lang]) return entry[lang];
        if (entry.pt) return entry.pt;
    }
    return key;
}
