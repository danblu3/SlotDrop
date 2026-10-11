// Console and UI icons, drawn here so the app never fetches anything from the web.
// These are fixed strings written by us, so it's fine to put them in innerHTML.
"use strict";

const PLATFORM_ICONS = {
  // Drawn after the real consoles, in their own colours, like RomM's icons.
  GBA: `<svg viewBox="0 0 72 44" aria-hidden="true">
    <path fill="#3d3893" d="M15 2h42c3 0 5 1 6 3l1 2H8l1-2c1-2 3-3 6-3z"/>
    <path fill="#5650b8" d="M14 4h44c8 0 13 6 13 17 0 13-5 21-13 21H14C6 42 1 34 1 21 1 10 6 4 14 4z"/>
    <path fill="#6a64c8" d="M14 4h44c6 0 10 3 12 9H2c2-6 6-9 12-9z" opacity=".6"/>
    <rect fill="#24223d" x="19.5" y="7" width="33" height="26" rx="3"/>
    <rect fill="#a7c3cf" x="23.5" y="10" width="25" height="18" rx="1"/>
    <rect fill="#d3e4ea" x="23.5" y="10" width="25" height="5" rx="1" opacity=".5"/>
    <path fill="#2a2748" d="M8 17.5h3.5V14h3v3.5H18v3h-3.5V24h-3v-3.5H8z"/>
    <circle fill="#2a2748" cx="12" cy="31" r="1.2"/><circle fill="#2a2748" cx="12" cy="35.5" r="1.2"/>
    <circle fill="#8f88dc" stroke="#2f2b6b" stroke-width=".8" cx="63" cy="16" r="3.4"/>
    <circle fill="#8f88dc" stroke="#2f2b6b" stroke-width=".8" cx="56.5" cy="21" r="3.4"/>
    <g fill="#38347c"><circle cx="58" cy="31" r=".9"/><circle cx="61" cy="30" r=".9"/><circle cx="64" cy="29" r=".9"/><circle cx="59" cy="34" r=".9"/><circle cx="62" cy="33" r=".9"/><circle cx="65" cy="32" r=".9"/></g>
    <rect fill="#3d3893" x="29" y="36" width="14" height="1.6" rx=".8"/>
  </svg>`,
  GB: `<svg viewBox="0 0 40 64" aria-hidden="true">
    <path fill="#d2cec7" stroke="#a29d95" stroke-width=".8" d="M3 1h34c1.2 0 2 .8 2 2v47c0 7-5 12-14 12H3c-1.2 0-2-.8-2-2V3c0-1.2.8-2 2-2z"/>
    <path stroke="#a29d95" stroke-width=".7" d="M1.5 4.5h37M5 1.5v3M35 1.5v3"/>
    <path fill="#5d5f6f" d="M6 7h28c.8 0 1.5.7 1.5 1.5V26c0 3-2 5-5 5H6c-.8 0-1.5-.7-1.5-1.5v-21C4.5 7.7 5.2 7 6 7z"/>
    <circle fill="#d8262f" cx="7.6" cy="15" r="1"/>
    <rect fill="#9aa83f" x="10.5" y="10" width="19" height="17"/>
    <rect fill="#b3bf5a" x="10.5" y="10" width="19" height="4" opacity=".6"/>
    <rect fill="#2c2f8a" x="8" y="33.5" width="15" height="1.6" rx=".5"/>
    <path fill="#2c2c33" d="M5 41h3.5v-3.5h3V41H15v3h-3.5v3.5h-3V44H5z"/>
    <circle fill="#9e1f4f" cx="32" cy="39.5" r="3"/><circle fill="#9e1f4f" cx="25.5" cy="43" r="3"/>
    <rect fill="#8d8a85" x="13" y="51.5" width="5" height="1.8" rx=".9" transform="rotate(-25 15.5 52.4)"/>
    <rect fill="#8d8a85" x="19.5" y="51.5" width="5" height="1.8" rx=".9" transform="rotate(-25 22 52.4)"/>
    <path stroke="#8d8a85" stroke-width="1.1" stroke-linecap="round" d="M27 58l3-6M29.5 59l3-6M32 59.5l3-6M34.5 59l2.5-5"/>
  </svg>`,
  GBC: `<svg viewBox="0 0 40 64" aria-hidden="true">
    <path fill="#16999c" d="M5 1h30c2.2 0 4 1.8 4 4v44c0 9-7 14-19 14S1 58 1 49V5c0-2.2 1.8-4 4-4z"/>
    <path fill="#2bb5b6" d="M5 1h30c2.2 0 4 1.8 4 4v3H1V5c0-2.2 1.8-4 4-4z" opacity=".6"/>
    <path fill="#2b2e38" d="M6.5 5h27c1.4 0 2.5 1.1 2.5 2.5V27c0 2.8-2.2 5-5 5H6.5C5.1 32 4 30.9 4 29.5v-22C4 6.1 5.1 5 6.5 5z"/>
    <rect fill="#a9d4e6" x="9" y="8" width="22" height="18"/>
    <rect fill="#e2f1f6" x="9" y="8" width="22" height="5" opacity=".5"/>
    <rect fill="#e9eaf0" x="11" y="28.3" width="9" height="1.4" rx=".5"/>
    <g><rect fill="#e8473c" x="21" y="28.3" width="1.6" height="1.4"/><rect fill="#f2b632" x="23" y="28.3" width="1.6" height="1.4"/><rect fill="#45b649" x="25" y="28.3" width="1.6" height="1.4"/><rect fill="#3b82d6" x="27" y="28.3" width="1.6" height="1.4"/><rect fill="#9b59c6" x="29" y="28.3" width="1.6" height="1.4"/></g>
    <path fill="#1e2a30" d="M5 40h3.5v-3.5h3V40H15v3h-3.5v3.5h-3V43H5z"/>
    <circle fill="#1e2a30" cx="32" cy="38.5" r="3"/><circle fill="#1e2a30" cx="25.5" cy="42" r="3"/>
    <rect fill="#0f6e71" x="14.5" y="51" width="4.5" height="1.7" rx=".85"/><rect fill="#0f6e71" x="21" y="51" width="4.5" height="1.7" rx=".85"/>
    <g fill="#0f6e71"><circle cx="29" cy="55" r=".8"/><circle cx="32" cy="55" r=".8"/><circle cx="30.5" cy="57.5" r=".8"/><circle cx="33.5" cy="57.5" r=".8"/><circle cx="32" cy="60" r=".8"/></g>
  </svg>`,
  ALL: `<svg viewBox="0 0 40 40" aria-hidden="true">
    <rect class="body" x="3" y="3" width="15" height="15" rx="4"/><rect class="body" x="22" y="3" width="15" height="15" rx="4"/>
    <rect class="body" x="3" y="22" width="15" height="15" rx="4"/><rect class="body" x="22" y="22" width="15" height="15" rx="4"/>
  </svg>`,
};

const ICONS = {
  search: `<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/></svg>`,
  sd: `<svg viewBox="0 0 24 24"><path d="M7 2h8l5 5v13a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z"/><path d="M9 6v3M12 6v3M15 6v3"/></svg>`,
  cloud: `<svg viewBox="0 0 24 24"><path d="M7 18a5 5 0 0 1-.6-10A6 6 0 0 1 18 9a4.5 4.5 0 0 1-.5 9z"/></svg>`,
  plus: `<svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>`,
  check: `<svg viewBox="0 0 24 24"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>`,
  close: `<svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18"/></svg>`,
  wand: `<svg viewBox="0 0 24 24"><path d="M4 20L15 9M14 4v3M19 9h-3M18 5l-2 2M10 4l1 1"/></svg>`,
  lock: `<svg viewBox="0 0 24 24"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>`,
  gear: `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9 7 7M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1"/></svg>`,
  eject: `<svg viewBox="0 0 24 24"><path d="M12 5l7 8H5z"/><path d="M5 18h14"/></svg>`,
  trash: `<svg viewBox="0 0 24 24"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/></svg>`,
  image: `<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="M4 18l5-5 4 4 3-3 4 4"/></svg>`,
  warn: `<svg viewBox="0 0 24 24"><path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18v.5"/></svg>`,
};
