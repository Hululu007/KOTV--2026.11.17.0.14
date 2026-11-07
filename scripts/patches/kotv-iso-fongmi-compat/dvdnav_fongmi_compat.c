/* Compatibility symbols expected by FongMi/mpv (ada8002+) but not yet in
 * VideoLAN libdvdnav 7.0.0 releases. Implemented on top of the public API so
 * DVD/ISO playback stays enabled. Remove once upstream ships these symbols. */
#include <dvdnav/dvdnav.h>
#include <dvdnav/dvd_types.h>
#include <stdint.h>

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

/* title_layout.h APIs used by FongMi/mpv stream_dvdnav.c */

dvdnav_status_t dvdnav_get_next_block_with_wait(dvdnav_t *self, uint8_t *buf,
                                               int32_t *event, int32_t *len)
{
  /* Stock 7.0.0 has no wait-aware reader; drain handling still works via
   * DVDNAV_WAIT + dvdnav_wait_skip on the regular get_next_block path. */
  return dvdnav_get_next_block(self, buf, event, len);
}

int8_t dvdnav_get_active_logical_stream(dvdnav_t *self,
                                        dvdnav_stream_type_t type)
{
  if (!self)
    return -1;
  switch (type) {
  case DVD_AUDIO_STREAM:
    return dvdnav_get_active_audio_stream(self);
  case DVD_SUBTITLE_STREAM:
    return dvdnav_get_active_spu_stream(self);
  default:
    return -1;
  }
}

int8_t dvdnav_get_number_of_stream_attributes(dvdnav_t *self,
                                              dvdnav_stream_type_t type)
{
  /* FongMi iterates logical attribute slots (audio 0..7, sub 0..31), not the
   * sparse enabled-stream count from dvdnav_get_number_of_streams(). */
  (void)self;
  switch (type) {
  case DVD_AUDIO_STREAM:
    return 8;
  case DVD_SUBTITLE_STREAM:
    return 32;
  default:
    return -1;
  }
}
