.pragma library

// Base language is Portuguese, matching Dank Jobs' own convention (its
// backend already returns status words like "ativo"/"pausado" in pt-BR).
// tr() falls back to the pt key itself for any language with no entry.
var strings = {
    "próximo job": { en: "next job", es: "próximo trabajo", fr: "prochain job", de: "nächster Job", it: "prossimo job" },
    "sem jobs pendentes": { en: "no pending jobs", es: "sin trabajos pendientes", fr: "aucun job en attente", de: "keine anstehenden Jobs", it: "nessun job in sospeso" },
    "jobs pendentes": { en: "pending jobs", es: "trabajos pendientes", fr: "jobs en attente", de: "anstehende Jobs", it: "job in sospeso" },
    "nenhum job pendente": { en: "no pending jobs", es: "ningún trabajo pendiente", fr: "aucun job en attente", de: "kein anstehender Job", it: "nessun job in sospeso" },
    "atualizar": { en: "refresh", es: "actualizar", fr: "actualiser", de: "aktualisieren", it: "aggiorna" },
    "ativo": { en: "active", es: "activo", fr: "actif", de: "aktiv", it: "attivo" },
    "pausado": { en: "paused", es: "pausado", fr: "en pause", de: "pausiert", it: "in pausa" },
};

function tr(key, lang) {
    if (lang === "pt") return key;
    var entry = strings[key];
    if (entry && entry[lang]) return entry[lang];
    return key;
}
