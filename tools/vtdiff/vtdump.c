/* Feed a byte stream to libvterm at ROWSxCOLS and print the final screen in
 * a neutral text format that the Go dumpers also produce:
 *   cursor ROW COL
 *   R<row> <text with trailing blanks trimmed>
 *   S<row>,<col> fg bg attrs      (one line per cell whose style is not default)
 * Colours: d (default), iN (palette index N), #rrggbb (direct colour).
 * attrs: letters b i u r s (bold italic underline reverse strike), or '-'. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "vterm.h"

static void colour(const VTermColor *c, int isfg, char *out) {
  if (isfg && VTERM_COLOR_IS_DEFAULT_FG(c)) { strcpy(out, "d"); return; }
  if (!isfg && VTERM_COLOR_IS_DEFAULT_BG(c)) { strcpy(out, "D"); return; }
  if (VTERM_COLOR_IS_INDEXED(c)) { sprintf(out, "i%d", c->indexed.idx); return; }
  sprintf(out, "#%02x%02x%02x", c->rgb.red, c->rgb.green, c->rgb.blue);
}

static void put_utf8(unsigned int c, FILE *f) {
  if (c < 0x80) fputc(c, f);
  else if (c < 0x800) { fputc(0xC0 | (c >> 6), f); fputc(0x80 | (c & 0x3F), f); }
  else if (c < 0x10000) { fputc(0xE0 | (c >> 12), f); fputc(0x80 | ((c >> 6) & 0x3F), f); fputc(0x80 | (c & 0x3F), f); }
  else { fputc(0xF0 | (c >> 18), f); fputc(0x80 | ((c >> 12) & 0x3F), f); fputc(0x80 | ((c >> 6) & 0x3F), f); fputc(0x80 | (c & 0x3F), f); }
}

int main(int argc, char **argv) {
  if (argc < 4) { fprintf(stderr, "usage: vtdump ROWS COLS FILE\n"); return 2; }
  int rows = atoi(argv[1]), cols = atoi(argv[2]);
  FILE *f = fopen(argv[3], "rb");
  if (!f) { perror(argv[3]); return 1; }
  VTerm *vt = vterm_new(rows, cols);
  vterm_set_utf8(vt, 1);
  VTermScreen *scr = vterm_obtain_screen(vt);
  vterm_screen_enable_altscreen(scr, 1);
  vterm_screen_reset(scr, 1);
  char buf[65536]; size_t n;
  while ((n = fread(buf, 1, sizeof buf, f)) > 0) vterm_input_write(vt, buf, n);
  VTermPos cur; vterm_state_get_cursorpos(vterm_obtain_state(vt), &cur);
  printf("cursor %d %d\n", cur.row, cur.col);
  for (int r = 0; r < rows; r++) {
    char line[65536]; size_t len = 0, keep = 0;
    FILE *m = fmemopen(line, sizeof line, "w");
    for (int c = 0; c < cols; c++) {
      VTermScreenCell cell; VTermPos p = { r, c };
      vterm_screen_get_cell(scr, p, &cell);
      if (cell.chars[0] == (uint32_t)-1) { fputs("\xc2\xa4", m); fflush(m); keep = ftell(m); continue; } /* right half of a wide char, printed as a currency sign */
      if (cell.chars[0] == 0) fputc(' ', m);
      else for (int k = 0; k < VTERM_MAX_CHARS_PER_CELL && cell.chars[k]; k++) put_utf8(cell.chars[k], m);
      fflush(m); len = ftell(m);
      if (cell.chars[0] != 0 && cell.chars[0] != ' ') keep = len;
    }
    fclose(m); line[keep] = 0;
    printf("R%d %s\n", r, line);
  }
  for (int r = 0; r < rows; r++) for (int c = 0; c < cols; c++) {
    VTermScreenCell cell; VTermPos p = { r, c };
    vterm_screen_get_cell(scr, p, &cell);
    if (cell.chars[0] == (uint32_t)-1) continue;
    char fg[16], bg[16], at[8] = {0}; int k = 0;
    colour(&cell.fg, 1, fg); colour(&cell.bg, 0, bg);
    if (cell.attrs.bold) at[k++] = 'b';
    if (cell.attrs.italic) at[k++] = 'i';
    if (cell.attrs.underline) at[k++] = 'u';
    if (cell.attrs.reverse) at[k++] = 'r';
    if (cell.attrs.strike) at[k++] = 's';
    if (!k) at[k++] = '-';
    if (strcmp(fg, "d") || strcmp(bg, "D") || at[0] != '-')
      printf("S%d,%d %s %s %s\n", r, c, fg, bg, at);
  }
  vterm_free(vt);
  return 0;
}
