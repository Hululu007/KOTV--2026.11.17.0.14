/* Compatibility symbols expected by FongMi/mpv (ada8002+) but not yet in
 * VideoLAN libdvdnav 7.0.0 releases. Implemented on top of the public API so
 * DVD/ISO playback stays enabled. Remove once upstream ships these symbols. */
#include <dvdnav/dvdnav.h>
#include <dvdnav/dvd_types.h>

#ifndef DVDNAV_HAVE_TIME_SEARCH_FLOOR
#define DVDNAV_HAVE_TIME_SEARCH_FLOOR 1
#endif

dvdnav_status_t dvdnav_menu_available(dvdnav_t *self, DVDMenuID_t menu)
{
  if (!self)
    return DVDNAV_STATUS_ERR;
  /* Released libdvdnav has no probe helper; Root/Title menus are the ones
   * FongMi/mpv queries to decide whether disc menus are usable. */
  switch (menu) {
  case DVD_MENU_Root:
  case DVD_MENU_Title:
    return DVDNAV_STATUS_OK;
  default:
    return DVDNAV_STATUS_ERR;
  }
}

dvdnav_status_t dvdnav_time_search_floor(dvdnav_t *self, uint64_t time)
{
  if (!self)
    return DVDNAV_STATUS_ERR;
  /* Prefer 7.x sector-by-time with mode -1 (Before = floor onto verified VOBU). */
#if defined(DVDNAV_VERSION) && DVDNAV_VERSION >= DVDNAV_VERSION_CODE(7, 0, 0)
  if (dvdnav_jump_to_sector_by_time(self, time, -1) == DVDNAV_STATUS_OK)
    return DVDNAV_STATUS_OK;
#endif
  return dvdnav_time_search(self, time);
}
