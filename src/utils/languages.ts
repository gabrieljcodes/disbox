export interface LanguageDefinition {
  code: string;
  name: string;
  flag: string;
  aliases: string[];
}

export const WORLD_LANGUAGES: LanguageDefinition[] = [
  { code: 'pt_br', name: 'Portuguese (Brazil) / Dublado', flag: '🇧🇷', aliases: ['portuguese (brazil)', 'brazilian', 'pt-br', 'pob', 'dublado', 'nacional'] },
  { code: 'pt_pt', name: 'Portuguese (Portugal)', flag: '🇵🇹', aliases: ['portuguese (portugal)', 'pt-pt', 'por', 'portuguese'] },
  { code: 'en', name: 'English', flag: '🇺🇸', aliases: ['english', 'eng', 'en'] },
  { code: 'ja', name: 'Japanese', flag: '🇯🇵', aliases: ['japanese', 'jap', 'jpn', 'ja'] },
  { code: 'es', name: 'Spanish / Latino', flag: '🇪🇸', aliases: ['spanish', 'castellano', 'latino', 'spa', 'es'] },
  { code: 'fr', name: 'French / Français', flag: '🇫🇷', aliases: ['french', 'francais', 'fra', 'fre', 'vff', 'vfq', 'fr'] },
  { code: 'de', name: 'German / Deutsch', flag: '🇩🇪', aliases: ['german', 'deutsch', 'ger', 'deu', 'de'] },
  { code: 'it', name: 'Italian / Italiano', flag: '🇮🇹', aliases: ['italian', 'italiano', 'ita', 'it'] },
  { code: 'ru', name: 'Russian / Русский', flag: '🇷🇺', aliases: ['russian', 'rus', 'ru'] },
  { code: 'ko', name: 'Korean / 한국어', flag: '🇰🇷', aliases: ['korean', 'kor', 'ko'] },
  { code: 'zh', name: 'Chinese / 中文', flag: '🇨🇳', aliases: ['chinese', 'mandarin', 'cantonese', 'chi', 'zho', 'zh'] },
  { code: 'hi', name: 'Hindi / हिन्दी', flag: '🇮🇳', aliases: ['hindi', 'hin', 'hi'] },
  { code: 'tr', name: 'Turkish / Türkçe', flag: '🇹🇷', aliases: ['turkish', 'tur', 'tr'] },
  { code: 'ar', name: 'Arabic / العربية', flag: '🇸🇦', aliases: ['arabic', 'ara', 'ar'] },
  { code: 'pl', name: 'Polish / Polski', flag: '🇵🇱', aliases: ['polish', 'pol', 'pl'] },
  { code: 'nl', name: 'Dutch / Nederlands', flag: '🇳🇱', aliases: ['dutch', 'nld', 'dut', 'nl'] },
  { code: 'sv', name: 'Swedish / Svenska', flag: '🇸🇪', aliases: ['swedish', 'swe', 'sv'] },
  { code: 'no', name: 'Norwegian / Norsk', flag: '🇳🇴', aliases: ['norwegian', 'nor', 'no'] },
  { code: 'da', name: 'Danish / Dansk', flag: '🇩🇰', aliases: ['danish', 'dan', 'da'] },
  { code: 'fi', name: 'Finnish / Suomi', flag: '🇫🇮', aliases: ['finnish', 'fin', 'fi'] },
  { code: 'uk', name: 'Ukrainian / Українська', flag: '🇺🇦', aliases: ['ukrainian', 'ukr', 'uk'] },
  { code: 'cs', name: 'Czech / Čeština', flag: '🇨🇿', aliases: ['czech', 'ces', 'cze', 'cs'] },
  { code: 'el', name: 'Greek / Ελληνικά', flag: '🇬🇷', aliases: ['greek', 'ell', 'el'] },
  { code: 'he', name: 'Hebrew / עברית', flag: '🇮🇱', aliases: ['hebrew', 'heb', 'he'] },
  { code: 'th', name: 'Thai / ไทย', flag: '🇹🇭', aliases: ['thai', 'tha', 'th'] },
  { code: 'vi', name: 'Vietnamese / Tiếng Việt', flag: '🇻🇳', aliases: ['vietnamese', 'vie', 'vi'] },
  { code: 'id', name: 'Indonesian / Bahasa', flag: '🇮🇩', aliases: ['indonesian', 'ind', 'id'] },
  { code: 'hu', name: 'Hungarian / Magyar', flag: '🇭🇺', aliases: ['hungarian', 'hun', 'hu'] },
  { code: 'ro', name: 'Romanian / Română', flag: '🇷🇴', aliases: ['romanian', 'ron', 'rum', 'ro'] },
];

export function formatLanguageBadge(lang: string): { label: string; flag: string; code: string } {
  if (!lang) return { label: 'Unknown', flag: '🌐', code: 'unknown' };
  const l = lang.toLowerCase().trim();

  for (const def of WORLD_LANGUAGES) {
    if (def.aliases.some((alias) => l.includes(alias) || l === alias)) {
      let shortLabel = def.code.toUpperCase();
      if (def.code === 'pt_br') shortLabel = 'PT-BR';
      else if (def.code === 'pt_pt') shortLabel = 'PT-PT';
      return { label: shortLabel, flag: def.flag, code: def.code };
    }
  }

  // Fallback
  return { label: lang.length > 8 ? lang.slice(0, 7) + '…' : lang, flag: '🌐', code: l };
}

export function buildLanguageSelectOptions(selectedVal = 'all'): string {
  const options = [
    `<option value="all" ${selectedVal === 'all' ? 'selected' : ''}>🌐 All Languages (No filter by default)</option>`,
    `<optgroup label="Audio (Dublado / Native)">`,
    ...WORLD_LANGUAGES.map((l) => {
      const val = `${l.code}_audio`;
      const isSelected = selectedVal === val || (l.code === 'pt_br' && selectedVal === 'pt_audio') || (l.code === 'en' && selectedVal === 'en_audio') || (l.code === 'ja' && selectedVal === 'ja_audio') || (l.code === 'es' && selectedVal === 'es_audio');
      return `<option value="${val}" ${isSelected ? 'selected' : ''}>${l.flag} ${l.name}</option>`;
    }),
    `</optgroup>`,
    `<optgroup label="Subtitles (Legendas)">`,
    ...WORLD_LANGUAGES.slice(0, 10).map((l) => {
      const val = `${l.code}_sub`;
      const isSelected = selectedVal === val || (l.code === 'pt_br' && selectedVal === 'pt_sub');
      return `<option value="${val}" ${isSelected ? 'selected' : ''}>💬 ${l.flag} ${l.name.split('/')[0].trim()} (Subtitles)</option>`;
    }),
    `</optgroup>`,
    `<optgroup label="Multi / Dual Audio">`,
    `<option value="dual_audio" ${selectedVal === 'dual_audio' || selectedVal === 'dual' ? 'selected' : ''}>🔊 Dual / Multi Audio</option>`,
    `</optgroup>`,
  ];

  return options.join('\n');
}
