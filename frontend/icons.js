// Console and UI icons, drawn here so the app never fetches anything from the web.
// These are fixed strings written by us, so it's fine to put them in innerHTML.
"use strict";

const PLATFORM_ICONS = {
  GBA: `<svg viewBox="0 0 64 40" aria-hidden="true">
    <path class="body" d="M12 4h40c6 0 9 3 10 8l1 16c0 6-3 9-9 9H10c-6 0-9-3-9-9l1-16c1-5 4-8 10-8z"/>
    <rect class="bezel" x="17" y="7" width="30" height="24" rx="3"/>
    <rect class="screen" x="21" y="10" width="22" height="16" rx="1"/>
    <path class="btn" d="M8 16h3v-3h3v3h3v3h-3v3h-3v-3H8z"/>
    <circle class="btn" cx="54" cy="15" r="2.6"/><circle class="btn" cx="50" cy="21" r="2.6"/>
  </svg>`,
  GB: `<svg viewBox="0 0 40 60" aria-hidden="true">
    <path class="body" d="M4 2h32c1.5 0 2 .5 2 2v44c0 6-4 10-12 10H4c-1.5 0-2-.5-2-2V4c0-1.5.5-2 2-2z"/>
    <rect class="bezel" x="6" y="6" width="28" height="22" rx="2" />
    <rect class="screen" x="10" y="9" width="20" height="16" rx="1"/>
    <path class="btn" d="M7 38h3v-3h3v3h3v3h-3v3h-3v-3H7z"/>
    <circle class="btn" cx="31" cy="36" r="2.6"/><circle class="btn" cx="25" cy="40" r="2.6"/>
    <path class="slot" d="M13 50l3 1M19 50l3 1"/>
  </svg>`,
  GBC: `<svg viewBox="0 0 40 60" aria-hidden="true">
    <path class="body" d="M7 2h26c3 0 5 2 5 5v40c0 7-5 11-12 11H14C7 58 2 54 2 47V7c0-3 2-5 5-5z"/>
    <rect class="bezel" x="6" y="6" width="28" height="23" rx="3"/>
    <rect class="screen" x="10" y="9" width="20" height="16" rx="1"/>
    <path class="btn" d="M7 39h3v-3h3v3h3v3h-3v3h-3v-3H7z"/>
    <circle class="btn" cx="31" cy="37" r="2.6"/><circle class="btn" cx="25" cy="41" r="2.6"/>
    <path class="slot" d="M14 51l3 1M20 51l3 1"/>
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
