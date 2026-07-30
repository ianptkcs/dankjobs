.pragma library

// Base language is Portuguese, matching Dank Jobs' own convention (its
// backend already returns status words like "ativo"/"pausado" in pt-BR).
// tr() falls back to the pt key itself for any language with no entry.
var strings = {
    "próximo job": { en: "next job" },
    "sem jobs pendentes": { en: "no pending jobs" },
    "jobs pendentes": { en: "pending jobs" },
    "nenhum job pendente": { en: "no pending jobs" },
    "atualizar": { en: "refresh" },
    "ativo": { en: "active" },
    "pausado": { en: "paused" },
};

function tr(key, lang) {
    if (lang === "pt") return key;
    var entry = strings[key];
    if (entry && entry[lang]) return entry[lang];
    return key;
}
