/*

MIT/X Consortium License

© 2020 Benny Daon <benny at codemadness dot org>
© 2014-2020 Hiltjo Posthuma <hiltjo at codemadness dot org>
© 2018 Devin J. Pohly <djpohly at gmail dot com>
© 2014-2017 Quentin Rameau <quinq at fifth dot space>
© 2009-2012 Aurélien APTEL <aurelien dot aptel at gmail dot com>
© 2008-2017 Anselm R Garbe <garbeam at gmail dot com>
© 2012-2017 Roberto E. Vargas Caballero <k0ga at shike2 dot com>
© 2012-2016 Christoph Lohmann <20h at r-36 dot net>
© 2013 Eon S. Jeon <esjeon at hyunmu dot am>
© 2013 Alexander Sedov <alex0player at gmail dot com>
© 2013 Mark Edgar <medgar123 at gmail dot com>
© 2013-2014 Eric Pruitt <eric.pruitt at gmail dot com>
© 2013 Michael Forney <mforney at mforney dot org>
© 2013-2014 Markus Teich <markus dot teich at stusta dot mhn dot de>
© 2014-2015 Laslo Hunhold <dev at frign dot de>

Permission is hereby granted, free of charge, to any person obtaining a
copy of this software and associated documentation files (the "Software"),
to deal in the Software without restriction, including without limitation
the rights to use, copy, modify, merge, publish, distribute, sublicense,
and/or sell copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL
THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
DEALINGS IN THE SOFTWARE.

*/ 
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <pwd.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <sys/ioctl.h>
#include <sys/select.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <termios.h>
#include <unistd.h>
#include <wchar.h>

#include "config.h"
#include "st.h"
#include "win.h"

#if   defined(__linux)
 #include <pty.h>
#elif defined(__OpenBSD__) || defined(__NetBSD__) || defined(__APPLE__)
 #include <util.h>
#elif defined(__FreeBSD__) || defined(__DragonFly__)
 #include <libutil.h>
#endif

extern void goSTDumpCB(char *, int, void *);
/* TODO: dummy function tobe developed or deleted */
void xclipcopy(void) {}
void xloadcols(void) {}
void xsetsel(char *str) {}
int xsetcolorname(int x, const char *name) { return 0; }
int xsetcursor(int cursor) { return 0; }
void xsettitle(char *p) {}
void xsetpointermotion(int set) {}
void xsetmode(int set, unsigned int flags) {}

/* Arbitrary sizes */
#define UTF_INVALID   0xFFFD
#define UTF_SIZ       4
#define ESC_BUF_SIZ   (128*UTF_SIZ)
#define ESC_ARG_SIZ   16
#define STR_BUF_SIZ   ESC_BUF_SIZ
#define STR_ARG_SIZ   ESC_ARG_SIZ

/* macros */
#define IS_SET(flag)		((t->mode & (flag)) != 0)
#define ISCONTROLC0(c)		(BETWEEN(c, 0, 0x1f) || (c) == 0x7f)
#define ISCONTROLC1(c)		(BETWEEN(c, 0x80, 0x9f))
#define ISCONTROL(c)		(ISCONTROLC0(c) || ISCONTROLC1(c))
#define ISDELIM(u)		(u && wcschr(worddelimiters, u))

enum term_mode {
	MODE_WRAP        = 1 << 0,
	MODE_INSERT      = 1 << 1,
	MODE_ALTSCREEN   = 1 << 2,
	MODE_CRLF        = 1 << 3,
	MODE_ECHO        = 1 << 4,
	MODE_PRINT       = 1 << 5,
	MODE_UTF8        = 1 << 6,
};

enum cursor_movement {
	CURSOR_SAVE,
	CURSOR_LOAD
};

enum cursor_state {
	CURSOR_DEFAULT  = 0,
	CURSOR_WRAPNEXT = 1,
	CURSOR_ORIGIN   = 2
};

enum charset {
	CS_GRAPHIC0,
	CS_GRAPHIC1,
	CS_UK,
	CS_USA,
	CS_MULTI,
	CS_GER,
	CS_FIN
};

enum escape_state {
	ESC_START      = 1,
	ESC_CSI        = 2,
	ESC_STR        = 4,  /* DCS, OSC, PM, APC */
	ESC_ALTCHARSET = 8,
	ESC_STR_END    = 16, /* a final string was encountered */
	ESC_TEST       = 32, /* Enter in test mode */
	ESC_UTF8       = 64,
};

typedef struct {
	int mode;
	int type;
	int snap;
	/*
	 * Selection variables:
	 * nb – normalized coordinates of the beginning of the selection
	 * ne – normalized coordinates of the end of the selection
	 * ob – original coordinates of the beginning of the selection
	 * oe – original coordinates of the end of the selection
	 */
	struct {
		int x, y;
	} nb, ne, ob, oe;

	int alt;
} Selection;

/* CSI Escape sequence structs */
/* ESC '[' [[ [<priv>] <arg> [;]] <mode> [<mode>]] */
typedef struct {
	char buf[ESC_BUF_SIZ]; /* raw string */
	size_t len;            /* raw string length */
	char priv;
	int arg[ESC_ARG_SIZ];
	int narg;              /* nb of args */
	char mode[2];
} CSIEscape;

/* STR Escape sequence structs */
/* ESC type [[ [<priv>] <arg> [;]] <mode>] ESC '\' */
typedef struct {
	char type;             /* ESC type ... */
	char *buf;             /* allocated raw string */
	size_t siz;            /* allocation size */
	size_t len;            /* raw string length */
	char *args[STR_ARG_SIZ];
	int narg;              /* nb of args */
} STREscape;

static void csidump(void);
static void csihandle(Term *);
static void csiparse(void);
static void csireset(void);
static int eschandle(Term*, uchar);
static void strdump(void);
static void strhandle(Term *);
static void strparse(void);
static void strreset(void);

static void tprinter(char *, size_t);
static void tdumpsel(void);
static void tdumpline(Term *, int);
static void tclearregion(Term *, int, int, int, int);
static void tcursor(Term *, int);
static void tdeletechar(Term *,int);
static void tdeleteline(Term *, int);
static void tinsertblank(Term *, int);
static void tinsertblankline(Term *, int);
static int tlinelen(Term *, int);
static void tmoveto(Term *, int, int);
static void tmoveato(Term *, int, int);
static void tnewline(Term *, int);
static void tputtab(Term *, int);
static void treset(Term *);
static void tscrollup(Term *, int, int);
static void tscrolldown(Term *, int, int);
static void tsetattr(Term *, int *, int);
static void tsetchar(Term *, Rune, Glyph *, int, int);
static void tsetdirt(Term *, int, int);
static void tsetscroll(Term *, int, int);
static void tswapscreen(Term *);
static void tsetmode(Term *, int, int, int *, int);
static void tfulldirt(Term *);
static void tcontrolcode(Term*, uchar);
static void tdectest(Term *, char );
static void tdefutf8(Term *, char);
static int32_t tdefcolor(int *, int *, int);
static void tdeftran(Term *, char);
static void tstrsequence(Term *, uchar);

static void selscroll(int, int);

static size_t utf8decode(const char *, Rune *, size_t);
static Rune utf8decodebyte(char, size_t *);
static char utf8encodebyte(Rune, size_t);
static size_t utf8validate(Rune *, size_t);

static char *base64dec(const char *);
static char base64dec_getc(const char **);

static ssize_t xwrite(int, const char *, size_t);

/* Globals */
static Selection sel;
static CSIEscape csiescseq;
static STREscape strescseq;
static int iofd = 1;
static int cmdfd;

static uchar utfbyte[UTF_SIZ + 1] = {0x80,    0, 0xC0, 0xE0, 0xF0};
static uchar utfmask[UTF_SIZ + 1] = {0xC0, 0x80, 0xE0, 0xF0, 0xF8};
static Rune utfmin[UTF_SIZ + 1] = {       0,    0,  0x80,  0x800,  0x10000};
static Rune utfmax[UTF_SIZ + 1] = {0x10FFFF, 0x7F, 0x7FF, 0xFFFF, 0x10FFFF};

ssize_t
xwrite(int fd, const char *s, size_t len)
{
	size_t aux = len;
	ssize_t r;

	while (len > 0) {
		r = write(fd, s, len);
		if (r < 0)
			return r;
		len -= r;
		s += r;
	}

	return aux;
}

void *
xmalloc(size_t len)
{
	void *p;

	if (!(p = malloc(len)))
		die("malloc: %s\n", strerror(errno));

	return p;
}

void *
xrealloc(void *p, size_t len)
{
    if ((p = realloc(p, len)) == NULL)
        die("realloc: %s\n", strerror(errno));

	return p;
}

char *
xstrdup(char *s)
{
	if ((s = strdup(s)) == NULL)
		die("strdup: %s\n", strerror(errno));

	return s;
}

size_t
utf8decode(const char *c, Rune *u, size_t clen)
{
	size_t i, j, len, type;
	Rune udecoded;

	*u = UTF_INVALID;
	if (!clen)
		return 0;
	udecoded = utf8decodebyte(c[0], &len);
	if (!BETWEEN(len, 1, UTF_SIZ))
		return 1;
	for (i = 1, j = 1; i < clen && j < len; ++i, ++j) {
		udecoded = (udecoded << 6) | utf8decodebyte(c[i], &type);
		if (type != 0)
			return j;
	}
	if (j < len)
		return 0;
	*u = udecoded;
	utf8validate(u, len);

	return len;
}

Rune
utf8decodebyte(char c, size_t *i)
{
	for (*i = 0; *i < LEN(utfmask); ++(*i))
		if (((uchar)c & utfmask[*i]) == utfbyte[*i])
			return (uchar)c & ~utfmask[*i];

	return 0;
}

size_t
utf8encode(Rune u, char *c)
{
	size_t len, i;

	len = utf8validate(&u, 0);
	if (len > UTF_SIZ)
		return 0;

	for (i = len - 1; i != 0; --i) {
		c[i] = utf8encodebyte(u, 0);
		u >>= 6;
	}
	c[0] = utf8encodebyte(u, len);

	return len;
}

char
utf8encodebyte(Rune u, size_t i)
{
	return utfbyte[i] | (u & ~utfmask[i]);
}

size_t
utf8validate(Rune *u, size_t i)
{
	if (!BETWEEN(*u, utfmin[i], utfmax[i]) || BETWEEN(*u, 0xD800, 0xDFFF))
		*u = UTF_INVALID;
	for (i = 1; *u > utfmax[i]; ++i)
		;

	return i;
}

static const char base64_digits[] = {
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 62, 0, 0, 0,
	63, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 0, 0, 0, -1, 0, 0, 0, 0, 1,
	2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24, 25, 0, 0, 0, 0, 0, 0, 26, 27, 28, 29, 30, 31, 32, 33, 34,
	35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
};

char
base64dec_getc(const char **src)
{
	while (**src && !isprint(**src))
		(*src)++;
	return **src ? *((*src)++) : '=';  /* emulate padding if string ends */
}

char *
base64dec(const char *src)
{
	size_t in_len = strlen(src);
	char *result, *dst;

	if (in_len % 4)
		in_len += 4 - (in_len % 4);
	result = dst = xmalloc(in_len / 4 * 3 + 1);
	while (*src) {
		int a = base64_digits[(unsigned char) base64dec_getc(&src)];
		int b = base64_digits[(unsigned char) base64dec_getc(&src)];
		int c = base64_digits[(unsigned char) base64dec_getc(&src)];
		int d = base64_digits[(unsigned char) base64dec_getc(&src)];

		/* invalid input. 'a' can be -1, e.g. if src is "\n" (c-str) */
		if (a == -1 || b == -1)
			break;

		*dst++ = (a << 2) | ((b & 0x30) >> 4);
		if (c == -1)
			break;
		*dst++ = ((b & 0x0f) << 4) | ((c & 0x3c) >> 2);
		if (d == -1)
			break;
		*dst++ = ((c & 0x03) << 6) | d;
	}
	*dst = '\0';
	return result;
}

void
selinit(void)
{
	sel.mode = SEL_IDLE;
	sel.snap = 0;
	sel.ob.x = -1;
}

int
tlinelen(Term *t, int y)
{
	int i = t->col;

	if (t->line[y][i - 1].mode & ATTR_WRAP)
		return i;

	while (i > 0 && t->line[y][i - 1].u == ' ')
		--i;

	return i;
}

int
selected(int x, int y)
{
    return 0;
}

void
selclear(void)
{
}

void
die(const char *errstr, ...)
{
	va_list ap;

	va_start(ap, errstr);
	vfprintf(stderr, errstr, ap);
	va_end(ap);
	exit(1);
}

void
ttywrite(Term *t, const char *s, size_t n, int may_echo)
{
    printf("ttywrite '%s' - doing nothing for now", s) ;
}


void
tsetdirt(Term *t, int top, int bot)
{
	int i;

	LIMIT(top, 0, t->row-1);
	LIMIT(bot, 0, t->row-1);

	for (i = top; i <= bot; i++)
		t->dirty[i] = 1;
}

void
tfulldirt(Term *t)
{
	tsetdirt(t, 0, t->row-1);
}

void
tcursor(Term *t, int mode)
{
	static TCursor c[2];
	int alt = IS_SET(MODE_ALTSCREEN);

	if (mode == CURSOR_SAVE) {
		c[alt] = t->c;
	} else if (mode == CURSOR_LOAD) {
		t->c = c[alt];
		tmoveto(t, c[alt].x, c[alt].y);
	}
}

void
treset(Term *t)
{
	uint i;

	t->c = (TCursor){{
		.mode = ATTR_NULL,
		.fg = defaultfg,
		.bg = defaultbg
	}, .x = 0, .y = 0, .state = CURSOR_DEFAULT};

	memset(t->tabs, 0, t->col * sizeof(*t->tabs));
	for (i = tabspaces; i < t->col; i += tabspaces)
		t->tabs[i] = 1;
	t->top = 0;
	t->bot = t->row - 1;
	t->mode = MODE_WRAP|MODE_UTF8;
	memset(t->trantbl, CS_USA, sizeof(t->trantbl));
	t->charset = 0;

	for (i = 0; i < 2; i++) {
		tmoveto(t, 0, 0);
		tcursor(t, CURSOR_SAVE);
		tclearregion(t, 0, 0, t->col-1, t->row-1);
		tswapscreen(t);
	}
}

Term *
tnew(int col, int row)
{
    Term *t = malloc(sizeof(Term));
    memset(t, 0, sizeof(Term));
    t->c.attr.fg = defaultfg;
    t->c.attr.bg = defaultbg;
  	tresize(t, col, row);
	treset(t);
    return t;
}

void
tswapscreen(Term *t)
{
	Line *tmp = t->line;

	t->line = t->alt;
	t->alt = tmp;
	t->mode ^= MODE_ALTSCREEN;
	tfulldirt(t);
}

void
tscrolldown(Term *t, int orig, int n)
{
	int i;
	Line temp;

	LIMIT(n, 0, t->bot-orig+1);

	tsetdirt(t, orig, t->bot-n);
	tclearregion(t, 0, t->bot-n+1, t->col-1, t->bot);

	for (i = t->bot; i >= orig+n; i--) {
		temp = t->line[i];
		t->line[i] = t->line[i-n];
		t->line[i-n] = temp;
	}

	selscroll(orig, n);
}

void
tscrollup(Term *t, int orig, int n)
{
	int i;
	Line temp;

	LIMIT(n, 0, t->bot-orig+1);

	tclearregion(t, 0, orig, t->col-1, orig+n-1);
	tsetdirt(t, orig+n, t->bot);

	for (i = orig; i <= t->bot-n; i++) {
		temp = t->line[i];
		t->line[i] = t->line[i+n];
		t->line[i+n] = temp;
	}

	selscroll(orig, -n);
}

void
selscroll(int orig, int n)
{
}

void
tnewline(Term *t, int first_col)
{
	int y = t->c.y;

	if (y == t->bot) {
		tscrollup(t, t->top, 1);
	} else {
		y++;
	}
	tmoveto(t, first_col ? 0 : t->c.x, y);
}

void
csiparse(void)
{
	char *p = csiescseq.buf, *np;
	long int v;

	csiescseq.narg = 0;
	if (*p == '?') {
		csiescseq.priv = 1;
		p++;
	}

	csiescseq.buf[csiescseq.len] = '\0';
	while (p < csiescseq.buf+csiescseq.len) {
		np = NULL;
		v = strtol(p, &np, 10);
		if (np == p)
			v = 0;
		if (v == LONG_MAX || v == LONG_MIN)
			v = -1;
		csiescseq.arg[csiescseq.narg++] = v;
		p = np;
		if (*p != ';' || csiescseq.narg == ESC_ARG_SIZ)
			break;
		p++;
	}
	csiescseq.mode[0] = *p++;
	csiescseq.mode[1] = (p < csiescseq.buf+csiescseq.len) ? *p : '\0';
}

/* for absolute user moves, when decom is set */
void
tmoveato(Term *t, int x, int y)
{
	tmoveto(t, x, y + ((t->c.state & CURSOR_ORIGIN) ? t->top: 0));
}

void
tmoveto(Term *t, int x, int y)
{
	int miny, maxy;

	if (t->c.state & CURSOR_ORIGIN) {
		miny = t->top;
		maxy = t->bot;
	} else {
		miny = 0;
		maxy = t->row - 1;
	}
	t->c.state &= ~CURSOR_WRAPNEXT;
	t->c.x = LIMIT(x, 0, t->col-1);
	t->c.y = LIMIT(y, miny, maxy);
}

void
tsetchar(Term *t, Rune u, Glyph *attr, int x, int y)
{
	static char *vt100_0[62] = { /* 0x41 - 0x7e */
		"↑", "↓", "→", "←", "█", "▚", "☃", /* A - G */
		0, 0, 0, 0, 0, 0, 0, 0, /* H - O */
		0, 0, 0, 0, 0, 0, 0, 0, /* P - W */
		0, 0, 0, 0, 0, 0, 0, " ", /* X - _ */
		"◆", "▒", "␉", "␌", "␍", "␊", "°", "±", /* ` - g */
		"␤", "␋", "┘", "┐", "┌", "└", "┼", "⎺", /* h - o */
		"⎻", "─", "⎼", "⎽", "├", "┤", "┴", "┬", /* p - w */
		"│", "≤", "≥", "π", "≠", "£", "·", /* x - ~ */
	};

	/*
	 * The table is proudly stolen from rxvt.
	 */
	if (t->trantbl[t->charset] == CS_GRAPHIC0 &&
	   BETWEEN(u, 0x41, 0x7e) && vt100_0[u - 0x41])
		utf8decode(vt100_0[u - 0x41], &u, UTF_SIZ);

	if (t->line[y][x].mode & ATTR_WIDE) {
		if (x+1 < t->col) {
			t->line[y][x+1].u = ' ';
			t->line[y][x+1].mode &= ~ATTR_WDUMMY;
		}
	} else if (t->line[y][x].mode & ATTR_WDUMMY) {
		t->line[y][x-1].u = ' ';
		t->line[y][x-1].mode &= ~ATTR_WIDE;
	}

	t->dirty[y] = 1;
	t->line[y][x] = *attr;
	t->line[y][x].u = u;
}

void
tclearregion(Term *t, int x1, int y1, int x2, int y2)
{
	int x, y, temp;
	Glyph *gp;

	if (x1 > x2)
		temp = x1, x1 = x2, x2 = temp;
	if (y1 > y2)
		temp = y1, y1 = y2, y2 = temp;

	LIMIT(x1, 0, t->col-1);
	LIMIT(x2, 0, t->col-1);
	LIMIT(y1, 0, t->row-1);
	LIMIT(y2, 0, t->row-1);

	for (y = y1; y <= y2; y++) {
		t->dirty[y] = 1;
		for (x = x1; x <= x2; x++) {
			gp = &t->line[y][x];
			if (selected(x, y))
				selclear();
			gp->fg = t->c.attr.fg;
			gp->bg = t->c.attr.bg;
			gp->mode = 0;
			gp->u = ' ';
		}
	}
}

void
tdeletechar(Term *t, int n)
{
	int dst, src, size;
	Glyph *line;


	LIMIT(n, 0, t->col - t->c.x);

	dst = t->c.x;
	src = t->c.x + n;
	size = t->col - src;
	line = t->line[t->c.y];

	memmove(&line[dst], &line[src], size * sizeof(Glyph));
	tclearregion(t, t->col-n, t->c.y, t->col-1, t->c.y);
}

void
tinsertblank(Term *t, int n)
{
	int dst, src, size;
	Glyph *line;

	LIMIT(n, 0, t->col - t->c.x);

	dst = t->c.x + n;
	src = t->c.x;
	size = t->col - dst;
	line = t->line[t->c.y];

	memmove(&line[dst], &line[src], size * sizeof(Glyph));
	tclearregion(t, src, t->c.y, dst - 1, t->c.y);
}

void
tinsertblankline(Term *t, int n)
{
	if (BETWEEN(t->c.y, t->top, t->bot))
		tscrolldown(t, t->c.y, n);
}

void
tdeleteline(Term *t, int n)
{
	if (BETWEEN(t->c.y, t->top, t->bot))
		tscrollup(t, t->c.y, n);
}

int32_t
tdefcolor(int *attr, int *npar, int l)
{
	int32_t idx = -1;
	uint r, g, b;

	switch (attr[*npar + 1]) {
	case 2: /* direct color in RGB space */
		if (*npar + 4 >= l) {
			fprintf(stderr,
				"erresc(38): Incorrect number of parameters (%d)\n",
				*npar);
			break;
		}
		r = attr[*npar + 2];
		g = attr[*npar + 3];
		b = attr[*npar + 4];
		*npar += 4;
		if (!BETWEEN(r, 0, 255) || !BETWEEN(g, 0, 255) || !BETWEEN(b, 0, 255))
			fprintf(stderr, "erresc: bad rgb color (%u,%u,%u)\n",
				r, g, b);
		else
			idx = TRUECOLOR(r, g, b);
		break;
	case 5: /* indexed color */
		if (*npar + 2 >= l) {
			fprintf(stderr,
				"erresc(38): Incorrect number of parameters (%d)\n",
				*npar);
			break;
		}
		*npar += 2;
		if (!BETWEEN(attr[*npar], 0, 255))
			fprintf(stderr, "erresc: bad fgcolor %d\n", attr[*npar]);
		else
			idx = attr[*npar];
		break;
	case 0: /* implemented defined (only foreground) */
	case 1: /* transparent */
	case 3: /* direct color in CMY space */
	case 4: /* direct color in CMYK space */
	default:
		fprintf(stderr,
		        "erresc(38): gfx attr %d unknown\n", attr[*npar]);
		break;
	}

	return idx;
}

void
tsetattr(Term *t, int *attr, int l)
{
	int i;
	int32_t idx;

	for (i = 0; i < l; i++) {
		switch (attr[i]) {
		case 0:
			t->c.attr.mode &= ~(
				ATTR_BOLD       |
				ATTR_FAINT      |
				ATTR_ITALIC     |
				ATTR_UNDERLINE  |
				ATTR_BLINK      |
				ATTR_REVERSE    |
				ATTR_INVISIBLE  |
				ATTR_STRUCK     );
			t->c.attr.fg = defaultfg;
			t->c.attr.bg = defaultbg;
			break;
		case 1:
			t->c.attr.mode |= ATTR_BOLD;
			break;
		case 2:
			t->c.attr.mode |= ATTR_FAINT;
			break;
		case 3:
			t->c.attr.mode |= ATTR_ITALIC;
			break;
		case 4:
			t->c.attr.mode |= ATTR_UNDERLINE;
			break;
		case 5: /* slow blink */
			/* FALLTHROUGH */
		case 6: /* rapid blink */
			t->c.attr.mode |= ATTR_BLINK;
			break;
		case 7:
			t->c.attr.mode |= ATTR_REVERSE;
			break;
		case 8:
			t->c.attr.mode |= ATTR_INVISIBLE;
			break;
		case 9:
			t->c.attr.mode |= ATTR_STRUCK;
			break;
		case 22:
			t->c.attr.mode &= ~(ATTR_BOLD | ATTR_FAINT);
			break;
		case 23:
			t->c.attr.mode &= ~ATTR_ITALIC;
			break;
		case 24:
			t->c.attr.mode &= ~ATTR_UNDERLINE;
			break;
		case 25:
			t->c.attr.mode &= ~ATTR_BLINK;
			break;
		case 27:
			t->c.attr.mode &= ~ATTR_REVERSE;
			break;
		case 28:
			t->c.attr.mode &= ~ATTR_INVISIBLE;
			break;
		case 29:
			t->c.attr.mode &= ~ATTR_STRUCK;
			break;
		case 38:
			if ((idx = tdefcolor(attr, &i, l)) >= 0)
				t->c.attr.fg = idx;
			break;
		case 39:
			t->c.attr.fg = defaultfg;
			break;
		case 48:
			if ((idx = tdefcolor(attr, &i, l)) >= 0)
				t->c.attr.bg = idx;
			break;
		case 49:
			t->c.attr.bg = defaultbg;
			break;
		default:
			if (BETWEEN(attr[i], 30, 37)) {
				t->c.attr.fg = attr[i] - 30;
			} else if (BETWEEN(attr[i], 40, 47)) {
				t->c.attr.bg = attr[i] - 40;
			} else if (BETWEEN(attr[i], 90, 97)) {
				t->c.attr.fg = attr[i] - 90 + 8;
			} else if (BETWEEN(attr[i], 100, 107)) {
				t->c.attr.bg = attr[i] - 100 + 8;
			} else {
				fprintf(stderr,
					"erresc(default): gfx attr %d unknown\n",
					attr[i]);
				csidump();
			}
			break;
		}
	}
}

void
tsetscroll(Term *t, int top, int b)
{
	int temp;

	LIMIT(top, 0, t->row-1);
	LIMIT(b,  0, t->row-1);
	if (top > b) {
		temp = top;
		top = b;
		b = temp;
	}
	t->top = top;
	t->bot = b;
}

void
tsetmode(Term *t, int priv, int set, int *args, int narg)
{
	int alt, *lim;

	for (lim = args + narg; args < lim; ++args) {
		if (priv) {
			switch (*args) {
			case 1: /* DECCKM -- Cursor key */
				xsetmode(set, MODE_APPCURSOR);
				break;
			case 5: /* DECSCNM -- Reverse video */
				xsetmode(set, MODE_REVERSE);
				break;
			case 6: /* DECOM -- Origin */
				MODBIT(t->c.state, set, CURSOR_ORIGIN);
				tmoveato(t, 0, 0);
				break;
			case 7: /* DECAWM -- Auto wrap */
				MODBIT(t->mode, set, MODE_WRAP);
				break;
			case 0:  /* Error (IGNORED) */
			case 2:  /* DECANM -- ANSI/VT52 (IGNORED) */
			case 3:  /* DECCOLM -- Column  (IGNORED) */
			case 4:  /* DECSCLM -- Scroll (IGNORED) */
			case 8:  /* DECARM -- Auto repeat (IGNORED) */
			case 18: /* DECPFF -- Printer feed (IGNORED) */
			case 19: /* DECPEX -- Printer extent (IGNORED) */
			case 42: /* DECNRCM -- National characters (IGNORED) */
			case 12: /* att610 -- Start blinking cursor (IGNORED) */
				break;
			case 25: /* DECTCEM -- Text Cursor Enable Mode */
				xsetmode(!set, MODE_HIDE);
				break;
			case 9:    /* X10 mouse compatibility mode */
				xsetpointermotion(0);
				xsetmode(0, MODE_MOUSE);
				xsetmode(set, MODE_MOUSEX10);
				break;
			case 1000: /* 1000: report button press */
				xsetpointermotion(0);
				xsetmode(0, MODE_MOUSE);
				xsetmode(set, MODE_MOUSEBTN);
				break;
			case 1002: /* 1002: report motion on button press */
				xsetpointermotion(0);
				xsetmode(0, MODE_MOUSE);
				xsetmode(set, MODE_MOUSEMOTION);
				break;
			case 1003: /* 1003: enable all mouse motions */
				xsetpointermotion(set);
				xsetmode(0, MODE_MOUSE);
				xsetmode(set, MODE_MOUSEMANY);
				break;
			case 1004: /* 1004: send focus events to tty */
				xsetmode(set, MODE_FOCUS);
				break;
			case 1006: /* 1006: extended reporting mode */
				xsetmode(set, MODE_MOUSESGR);
				break;
			case 1034:
				xsetmode(set, MODE_8BIT);
				break;
			case 1049: /* swap screen & set/restore cursor as xterm */
				if (!allowaltscreen)
					break;
				tcursor(t, (set) ? CURSOR_SAVE : CURSOR_LOAD);
				/* FALLTHROUGH */
			case 47: /* swap screen */
			case 1047:
				if (!allowaltscreen)
					break;
				alt = IS_SET(MODE_ALTSCREEN);
				if (alt) {
					tclearregion(t, 0, 0, t->col-1,
							t->row-1);
				}
				if (set ^ alt) /* set is always 1 or 0 */
					tswapscreen(t);
				if (*args != 1049)
					break;
				/* FALLTHROUGH */
			case 1048:
				tcursor(t, (set) ? CURSOR_SAVE : CURSOR_LOAD);
				break;
			case 2004: /* 2004: bracketed paste mode */
				xsetmode(set, MODE_BRCKTPASTE);
				break;
			/* Not implemented mouse modes. See comments there. */
			case 1001: /* mouse highlight mode; can hang the
				      terminal by design when implemented. */
			case 1005: /* UTF-8 mouse mode; will confuse
				      applications not supporting UTF-8
				      and luit. */
			case 1015: /* urxvt mangled mouse mode; incompatible
				      and can be mistaken for other control
				      codes. */
				break;
			default:
				fprintf(stderr,
					"erresc: unknown private set/reset mode %d\n",
					*args);
				break;
			}
		} else {
			switch (*args) {
			case 0:  /* Error (IGNORED) */
				break;
			case 2:
				xsetmode(set, MODE_KBDLOCK);
				break;
			case 4:  /* IRM -- Insertion-replacement */
				MODBIT(t->mode, set, MODE_INSERT);
				break;
			case 12: /* SRM -- Send/Receive */
				MODBIT(t->mode, !set, MODE_ECHO);
				break;
			case 20: /* LNM -- Linefeed/new line */
				MODBIT(t->mode, set, MODE_CRLF);
				break;
			default:
				fprintf(stderr,
					"erresc: unknown set/reset mode %d\n",
					*args);
				break;
			}
		}
	}
}

void
csihandle(Term *t)
{
	char buf[40];
	int len;

	switch (csiescseq.mode[0]) {
	default:
	unknown:
		fprintf(stderr, "erresc: unknown csi ");
		csidump();
		/* die(""); */
		break;
	case '@': /* ICH -- Insert <n> blank char */
		DEFAULT(csiescseq.arg[0], 1);
		tinsertblank(t, csiescseq.arg[0]);
		break;
	case 'A': /* CUU -- Cursor <n> Up */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, t->c.x, t->c.y-csiescseq.arg[0]);
		break;
	case 'B': /* CUD -- Cursor <n> Down */
	case 'e': /* VPR --Cursor <n> Down */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, t->c.x, t->c.y+csiescseq.arg[0]);
		break;
	case 'i': /* MC -- Media Copy */
		switch (csiescseq.arg[0]) {
		case 0:
			tdump();
			break;
		case 1:
			tdumpline(t, t->c.y);
			break;
		case 2:
			tdumpsel();
			break;
		case 4:
			t->mode &= ~MODE_PRINT;
			break;
		case 5:
			t->mode |= MODE_PRINT;
			break;
		}
		break;
	case 'c': /* DA -- Device Attributes */
		if (csiescseq.arg[0] == 0)
			ttywrite(t, vtiden, strlen(vtiden), 0);
		break;
	case 'b': /* REP -- if last char is printable print it <n> more times */
		DEFAULT(csiescseq.arg[0], 1);
		if (t->lastc)
			while (csiescseq.arg[0]-- > 0)
				tputc(t, t->lastc);
		break;
	case 'C': /* CUF -- Cursor <n> Forward */
	case 'a': /* HPR -- Cursor <n> Forward */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, t->c.x+csiescseq.arg[0], t->c.y);
		break;
	case 'D': /* CUB -- Cursor <n> Backward */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, t->c.x-csiescseq.arg[0], t->c.y);
		break;
	case 'E': /* CNL -- Cursor <n> Down and first col */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, 0, t->c.y+csiescseq.arg[0]);
		break;
	case 'F': /* CPL -- Cursor <n> Up and first col */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, 0, t->c.y-csiescseq.arg[0]);
		break;
	case 'g': /* TBC -- Tabulation clear */
		switch (csiescseq.arg[0]) {
		case 0: /* clear current tab stop */
			t->tabs[t->c.x] = 0;
			break;
		case 3: /* clear all the tabs */
			memset(t->tabs, 0, t->col * sizeof(*t->tabs));
			break;
		default:
			goto unknown;
		}
		break;
	case 'G': /* CHA -- Move to <col> */
	case '`': /* HPA */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveto(t, csiescseq.arg[0]-1, t->c.y);
		break;
	case 'H': /* CUP -- Move to <row> <col> */
	case 'f': /* HVP */
		DEFAULT(csiescseq.arg[0], 1);
		DEFAULT(csiescseq.arg[1], 1);
		tmoveato(t, csiescseq.arg[1]-1, csiescseq.arg[0]-1);
		break;
	case 'I': /* CHT -- Cursor Forward Tabulation <n> tab stops */
		DEFAULT(csiescseq.arg[0], 1);
		tputtab(t, csiescseq.arg[0]);
		break;
	case 'J': /* ED -- Clear screen */
		switch (csiescseq.arg[0]) {
		case 0: /* below */
			tclearregion(t, t->c.x, t->c.y, t->col-1, t->c.y);
			if (t->c.y < t->row-1) {
				tclearregion(t, 0, t->c.y+1, t->col-1,
						t->row-1);
			}
			break;
		case 1: /* above */
			if (t->c.y > 1)
				tclearregion(t, 0, 0, t->col-1, t->c.y-1);
			tclearregion(t, 0, t->c.y, t->c.x, t->c.y);
			break;
		case 2: /* all */
			tclearregion(t, 0, 0, t->col-1, t->row-1);
			break;
		default:
			goto unknown;
		}
		break;
	case 'K': /* EL -- Clear line */
		switch (csiescseq.arg[0]) {
		case 0: /* right */
			tclearregion(t, t->c.x, t->c.y, t->col-1,
					t->c.y);
			break;
		case 1: /* left */
			tclearregion(t, 0, t->c.y, t->c.x, t->c.y);
			break;
		case 2: /* all */
			tclearregion(t, 0, t->c.y, t->col-1, t->c.y);
			break;
		}
		break;
	case 'S': /* SU -- Scroll <n> line up */
		DEFAULT(csiescseq.arg[0], 1);
		tscrollup(t, t->top, csiescseq.arg[0]);
		break;
	case 'T': /* SD -- Scroll <n> line down */
		DEFAULT(csiescseq.arg[0], 1);
		tscrolldown(t, t->top, csiescseq.arg[0]);
		break;
	case 'L': /* IL -- Insert <n> blank lines */
		DEFAULT(csiescseq.arg[0], 1);
		tinsertblankline(t, csiescseq.arg[0]);
		break;
	case 'l': /* RM -- Reset Mode */
		tsetmode(t, csiescseq.priv, 0, csiescseq.arg, csiescseq.narg);
		break;
	case 'M': /* DL -- Delete <n> lines */
		DEFAULT(csiescseq.arg[0], 1);
		tdeleteline(t, csiescseq.arg[0]);
		break;
	case 'X': /* ECH -- Erase <n> char */
		DEFAULT(csiescseq.arg[0], 1);
		tclearregion(t, t->c.x, t->c.y,
				t->c.x + csiescseq.arg[0] - 1, t->c.y);
		break;
	case 'P': /* DCH -- Delete <n> char */
		DEFAULT(csiescseq.arg[0], 1);
		tdeletechar(t, csiescseq.arg[0]);
		break;
	case 'Z': /* CBT -- Cursor Backward Tabulation <n> tab stops */
		DEFAULT(csiescseq.arg[0], 1);
		tputtab(t, -csiescseq.arg[0]);
		break;
	case 'd': /* VPA -- Move to <row> */
		DEFAULT(csiescseq.arg[0], 1);
		tmoveato(t, t->c.x, csiescseq.arg[0]-1);
		break;
	case 'h': /* SM -- Set terminal mode */
		tsetmode(t, csiescseq.priv, 1, csiescseq.arg, csiescseq.narg);
		break;
	case 'm': /* SGR -- Terminal attribute (color) */
		tsetattr(t, csiescseq.arg, csiescseq.narg);
		break;
	case 'n': /* DSR – Device Status Report (cursor position) */
		if (csiescseq.arg[0] == 6) {
			len = snprintf(buf, sizeof(buf), "\033[%i;%iR",
					t->c.y+1, t->c.x+1);
			ttywrite(t, buf, len, 0);
		}
		break;
	case 'r': /* DECSTBM -- Set Scrolling Region */
		if (csiescseq.priv) {
			goto unknown;
		} else {
			DEFAULT(csiescseq.arg[0], 1);
			DEFAULT(csiescseq.arg[1], t->row);
			tsetscroll(t, csiescseq.arg[0]-1, csiescseq.arg[1]-1);
			tmoveato(t, 0, 0);
		}
		break;
	case 's': /* DECSC -- Save cursor position (ANSI.SYS) */
		tcursor(t, CURSOR_SAVE);
		break;
	case 'u': /* DECRC -- Restore cursor position (ANSI.SYS) */
		tcursor(t, CURSOR_LOAD);
		break;
	case ' ':
		switch (csiescseq.mode[1]) {
		case 'q': /* DECSCUSR -- Set Cursor Style */
			if (xsetcursor(csiescseq.arg[0]))
				goto unknown;
			break;
		default:
			goto unknown;
		}
		break;
	}
}

void
csidump(void)
{
	size_t i;
	uint c;

	fprintf(stderr, "ESC[");
	for (i = 0; i < csiescseq.len; i++) {
		c = csiescseq.buf[i] & 0xff;
		if (isprint(c)) {
			putc(c, stderr);
		} else if (c == '\n') {
			fprintf(stderr, "(\\n)");
		} else if (c == '\r') {
			fprintf(stderr, "(\\r)");
		} else if (c == 0x1b) {
			fprintf(stderr, "(\\e)");
		} else {
			fprintf(stderr, "(%02x)", c);
		}
	}
	putc('\n', stderr);
}

void
csireset(void)
{
	memset(&csiescseq, 0, sizeof(csiescseq));
}

void
strhandle(Term *t)
{
	char *p = NULL, *dec;
	int j, narg, par;

	t->esc &= ~(ESC_STR_END|ESC_STR);
	strparse();
	par = (narg = strescseq.narg) ? atoi(strescseq.args[0]) : 0;

	switch (strescseq.type) {
	case ']': /* OSC -- Operating System Command */
		switch (par) {
		case 0:
		case 1:
		case 2:
			if (narg > 1)
				xsettitle(strescseq.args[1]);
			return;
		case 52:
			if (narg > 2 && allowwindowops) {
				dec = base64dec(strescseq.args[2]);
				if (dec) {
					xsetsel(dec);
					xclipcopy();
				} else {
					fprintf(stderr, "erresc: invalid base64\n");
				}
			}
			return;
		case 4: /* color set */
			if (narg < 3)
				break;
			p = strescseq.args[2];
			/* FALLTHROUGH */
		case 104: /* color reset, here p = NULL */
			j = (narg > 1) ? atoi(strescseq.args[1]) : -1;
			if (xsetcolorname(j, p)) {
				if (par == 104 && narg <= 1)
					return; /* color reset without parameter */
				fprintf(stderr, "erresc: invalid color j=%d, p=%s\n",
				        j, p ? p : "(null)");
			} else {
				/*
				 * TODO : shoul;d we redraw?
				 * are dirty
				 */
				;
			}
			return;
		}
		break;
	case 'k': /* old title set compatibility */
		xsettitle(strescseq.args[0]);
		return;
	case 'P': /* DCS -- Device Control String */
	case '_': /* APC -- Application Program Command */
	case '^': /* PM -- Privacy Message */
		return;
	}

	fprintf(stderr, "erresc: unknown str ");
	strdump();
}

void
strparse(void)
{
	int c;
	char *p = strescseq.buf;

	strescseq.narg = 0;
	strescseq.buf[strescseq.len] = '\0';

	if (*p == '\0')
		return;

	while (strescseq.narg < STR_ARG_SIZ) {
		strescseq.args[strescseq.narg++] = p;
		while ((c = *p) != ';' && c != '\0')
			++p;
		if (c == '\0')
			return;
		*p++ = '\0';
	}
}

void
strdump(void)
{
	size_t i;
	uint c;

	fprintf(stderr, "ESC%c", strescseq.type);
	for (i = 0; i < strescseq.len; i++) {
		c = strescseq.buf[i] & 0xff;
		if (c == '\0') {
			putc('\n', stderr);
			return;
		} else if (isprint(c)) {
			putc(c, stderr);
		} else if (c == '\n') {
			fprintf(stderr, "(\\n)");
		} else if (c == '\r') {
			fprintf(stderr, "(\\r)");
		} else if (c == 0x1b) {
			fprintf(stderr, "(\\e)");
		} else {
			fprintf(stderr, "(%02x)", c);
		}
	}
	fprintf(stderr, "ESC\\\n");
}

void
strreset(void)
{
	strescseq = (STREscape){
		.buf = xrealloc(strescseq.buf, STR_BUF_SIZ),
		.siz = STR_BUF_SIZ,
	};
}

void
sendbreak(const Arg *arg)
{
	if (tcsendbreak(cmdfd, 0))
		perror("Error sending break");
}

void
tprinter(char *s, size_t len)
{
	if (iofd != -1 && xwrite(iofd, s, len) < 0) {
		perror("Error writing to output file");
		close(iofd);
		iofd = -1;
	}
}

void
tdumpsel(void)
{}

void
tdumpline(Term *t, int n)
{
	char buf[UTF_SIZ];
	Glyph *bp, *end;

	bp = &t->line[n][0];
	end = &bp[MIN(tlinelen(t, n), t->col) - 1];
	if (bp != end || bp->u != ' ') {
		for ( ; bp <= end; ++bp)
			tprinter(buf, utf8encode(bp->u, buf));
	}
	tprinter("\n", 1);
}

int
tdump2cb(Term *t, void *context) {
	int i, l =0, tot=0;
    char *outbuf, *out;
    outbuf = malloc(CHUNK_SIZE);
    out = outbuf;
    
	for (i = 0; i < t->row; ++i) {
        char buf[UTF_SIZ];
        Glyph *bp, *end;

        bp = &t->line[i][0];
        end = &bp[MIN(tlinelen(t, i), t->col) - 1];
        if (bp != end || bp->u != ' ') {
            for ( ; bp <= end; ++bp) {
                int ul = utf8encode(bp->u, buf);
                /* if the buffer is fulll call the cb and start fresh */
                if (l + ul > CHUNK_SIZE) {
                    goSTDumpCB(outbuf, l, context);
                    out = outbuf;
                    tot += l;
                    l = 0;
                }
                memcpy(out, buf, ul);
                l += ul;
                out += ul;
            }
        }
        /* if it's not the last row, add a new line */
        if (i + 1 != t->row) {
            *out = 10;
            out++;
            l++;
        }
    }
    /* send the last buffer */
    if (l > 0)
        goSTDumpCB(outbuf, l, context);
    free(outbuf);
    return tot+l;
}

int
tdump2buf(Term *t, char *out) {
	int i, l =0;

	for (i = 0; i < t->row; ++i) {
        char buf[UTF_SIZ];
        Glyph *bp, *end;

        bp = &t->line[i][0];
        end = &bp[MIN(tlinelen(t, i), t->col) - 1];
        if (bp != end || bp->u != ' ') {
            for ( ; bp <= end; ++bp) {
                int ul = utf8encode(bp->u, buf);
                memcpy(out, buf, ul);
                out += ul;
                l += ul;
            }
        }
        /* if it's nott the last row, add a new line */
        if (i + 1 != t->row) {
            *out = 10;
            out++;
            l++;
        }
    }
    return l;
}
void
tdump(Term *t)
{
	int i;

	for (i = 0; i < t->row; ++i) 
		tdumpline(t, i);
}

void
tputtab(Term *t, int n)
{
	uint x = t->c.x;

	if (n > 0) {
		while (x < t->col && n--)
			for (++x; x < t->col && !t->tabs[x]; ++x)
				/* nothing */ ;
	} else if (n < 0) {
		while (x > 0 && n++)
			for (--x; x > 0 && !t->tabs[x]; --x)
				/* nothing */ ;
	}
	t->c.x = LIMIT(x, 0, t->col-1);
}

void
tdefutf8(Term *t, char ascii)
{
	if (ascii == 'G')
		t->mode |= MODE_UTF8;
	else if (ascii == '@')
		t->mode &= ~MODE_UTF8;
}

void
tdeftran(Term *t, char ascii)
{
	static char cs[] = "0B";
	static int vcs[] = {CS_GRAPHIC0, CS_USA};
	char *p;

	if ((p = strchr(cs, ascii)) == NULL) {
		fprintf(stderr, "esc unhandled charset: ESC ( %c\n", ascii);
	} else {
		t->trantbl[t->icharset] = vcs[p - cs];
	}
}

void
tdectest(Term *t, char c)
{
	int x, y;

	if (c == '8') { /* DEC screen alignment test. */
		for (x = 0; x < t->col; ++x) {
			for (y = 0; y < t->row; ++y)
				tsetchar(t, 'E', &t->c.attr, x, y);
		}
	}
}

void
tstrsequence(Term *t, uchar c)
{

	switch (c) {
	case 0x90:   /* DCS -- Device Control String */
		c = 'P';
		break;
	case 0x9f:   /* APC -- Application Program Command */
		c = '_';
		break;
	case 0x9e:   /* PM -- Privacy Message */
		c = '^';
		break;
	case 0x9d:   /* OSC -- Operating System Command */
		c = ']';
		break;
	}
	strreset();
	strescseq.type = c;
	t->esc |= ESC_STR;
}

void
tcontrolcode(Term *t, uchar ascii)
{

	switch (ascii) {
	case '\t':   /* HT */
		tputtab(t, 1);
		return;
	case '\b':   /* BS */
		tmoveto(t, t->c.x-1, t->c.y);
		return;
	case '\r':   /* CR */
		tmoveto(t, 0, t->c.y);
		return;
	case '\f':   /* LF */
	case '\v':   /* VT */
	case '\n':   /* LF */
		/* go to first col if the mode is set */
		tnewline(t, IS_SET(MODE_CRLF));
		return;
	case '\a':   /* BEL */
		if (t->esc & ESC_STR_END) {
			/* backwards compatibility to xterm */
			strhandle(t);
		} else 
            /* TODO: ring the bell */
            ;
		break;
	case '\033': /* ESC */
		csireset();
		t->esc &= ~(ESC_CSI|ESC_ALTCHARSET|ESC_TEST);
		t->esc |= ESC_START;
		return;
	case '\016': /* SO (LS1 -- Locking shift 1) */
	case '\017': /* SI (LS0 -- Locking shift 0) */
		t->charset = 1 - (ascii - '\016');
		return;
	case '\032': /* SUB */
		tsetchar(t, '?', &t->c.attr, t->c.x, t->c.y);
		/* FALLTHROUGH */
	case '\030': /* CAN */
		csireset();
		break;
	case '\005': /* ENQ (IGNORED) */
	case '\000': /* NUL (IGNORED) */
	case '\021': /* XON (IGNORED) */
	case '\023': /* XOFF (IGNORED) */
	case 0177:   /* DEL (IGNORED) */
		return;
	case 0x80:   /* TODO: PAD */
	case 0x81:   /* TODO: HOP */
	case 0x82:   /* TODO: BPH */
	case 0x83:   /* TODO: NBH */
	case 0x84:   /* TODO: IND */
		break;
	case 0x85:   /* NEL -- Next line */
		tnewline(t, 1); /* always go to first col */
		break;
	case 0x86:   /* TODO: SSA */
	case 0x87:   /* TODO: ESA */
		break;
	case 0x88:   /* HTS -- Horizontal tab stop */
		t->tabs[t->c.x] = 1;
		break;
	case 0x89:   /* TODO: HTJ */
	case 0x8a:   /* TODO: VTS */
	case 0x8b:   /* TODO: PLD */
	case 0x8c:   /* TODO: PLU */
	case 0x8d:   /* TODO: RI */
	case 0x8e:   /* TODO: SS2 */
	case 0x8f:   /* TODO: SS3 */
	case 0x91:   /* TODO: PU1 */
	case 0x92:   /* TODO: PU2 */
	case 0x93:   /* TODO: STS */
	case 0x94:   /* TODO: CCH */
	case 0x95:   /* TODO: MW */
	case 0x96:   /* TODO: SPA */
	case 0x97:   /* TODO: EPA */
	case 0x98:   /* TODO: SOS */
	case 0x99:   /* TODO: SGCI */
		break;
	case 0x9a:   /* DECID -- Identify Terminal */
		ttywrite(t, vtiden, strlen(vtiden), 0);
		break;
	case 0x9b:   /* TODO: CSI */
	case 0x9c:   /* TODO: ST */
		break;
	case 0x90:   /* DCS -- Device Control String */
	case 0x9d:   /* OSC -- Operating System Command */
	case 0x9e:   /* PM -- Privacy Message */
	case 0x9f:   /* APC -- Application Program Command */
		tstrsequence(t, ascii);
		return;
	}
	/* only CAN, SUB, \a and C1 chars interrupt a sequence */
	t->esc &= ~(ESC_STR_END|ESC_STR);
}

/*
 * returns 1 when the sequence is finished and it hasn't to read
 * more characters for this sequence, otherwise 0
 */
int
eschandle(Term *t, uchar ascii)
{
	switch (ascii) {
	case '[':
		t->esc |= ESC_CSI;
		return 0;
	case '#':
		t->esc |= ESC_TEST;
		return 0;
	case '%':
		t->esc |= ESC_UTF8;
		return 0;
	case 'P': /* DCS -- Device Control String */
	case '_': /* APC -- Application Program Command */
	case '^': /* PM -- Privacy Message */
	case ']': /* OSC -- Operating System Command */
	case 'k': /* old title set compatibility */
		tstrsequence(t, ascii);
		return 0;
	case 'n': /* LS2 -- Locking shift 2 */
	case 'o': /* LS3 -- Locking shift 3 */
		t->charset = 2 + (ascii - 'n');
		break;
	case '(': /* GZD4 -- set primary charset G0 */
	case ')': /* G1D4 -- set secondary charset G1 */
	case '*': /* G2D4 -- set tertiary charset G2 */
	case '+': /* G3D4 -- set quaternary charset G3 */
		t->icharset = ascii - '(';
		t->esc |= ESC_ALTCHARSET;
		return 0;
	case 'D': /* IND -- Linefeed */
		if (t->c.y == t->bot) {
			tscrollup(t, t->top, 1);
		} else {
			tmoveto(t, t->c.x, t->c.y+1);
		}
		break;
	case 'E': /* NEL -- Next line */
		tnewline(t, 1); /* always go to first col */
		break;
	case 'H': /* HTS -- Horizontal tab stop */
		t->tabs[t->c.x] = 1;
		break;
	case 'M': /* RI -- Reverse index */
		if (t->c.y == t->top) {
			tscrolldown(t, t->top, 1);
		} else {
			tmoveto(t, t->c.x, t->c.y-1);
		}
		break;
	case 'Z': /* DECID -- Identify Terminal */
		ttywrite(t, vtiden, strlen(vtiden), 0);
		break;
	case 'c': /* RIS -- Reset to initial state */
		treset(t);
		resettitle();
		xloadcols();
		break;
	case '=': /* DECPAM -- Application keypad */
		xsetmode(1, MODE_APPKEYPAD);
		break;
	case '>': /* DECPNM -- Normal keypad */
		xsetmode(0, MODE_APPKEYPAD);
		break;
	case '7': /* DECSC -- Save Cursor */
		tcursor(t, CURSOR_SAVE);
		break;
	case '8': /* DECRC -- Restore Cursor */
		tcursor(t, CURSOR_LOAD);
		break;
	case '\\': /* ST -- String Terminator */
		if (t->esc & ESC_STR_END)
			strhandle(t);
		break;
	default:
		fprintf(stderr, "erresc: unknown sequence ESC 0x%02X '%c'\n",
			(uchar) ascii, isprint(ascii)? ascii:'.');
		break;
	}
	return 1;
}

void
tputc(Term *t, Rune u)
{
	char c[UTF_SIZ];
	int control;
	int width, len;
	Glyph *gp;

	control = ISCONTROL(u);
	if (u < 127 || !IS_SET(MODE_UTF8)) {
		c[0] = u;
		width = len = 1;
	} else {
		len = utf8encode(u, c);
		if (!control && (width = wcwidth(u)) == -1)
			width = 1;
	}

	if (IS_SET(MODE_PRINT))
		tprinter(c, len);

	/*
	 * STR sequence must be checked before anything else
	 * because it uses all following characters until it
	 * receives a ESC, a SUB, a ST or any other C1 control
	 * character.
	 */
	if (t->esc & ESC_STR) {
		if (u == '\a' || u == 030 || u == 032 || u == 033 ||
		   ISCONTROLC1(u)) {
			t->esc &= ~(ESC_START|ESC_STR);
			t->esc |= ESC_STR_END;
			goto check_control_code;
		}

		if (strescseq.len+len >= strescseq.siz) {
			/*
			 * Here is a bug in terminals. If the user never sends
			 * some code to stop the str or esc command, then st
			 * will stop responding. But this is better than
			 * silently failing with unknown characters. At least
			 * then users will report back.
			 *
			 * In the case users ever get fixed, here is the code:
			 */
			/*
			 * t->esc = 0;
			 * strhandle();
			 */
			if (strescseq.siz > (SIZE_MAX - UTF_SIZ) / 2)
				return;
			strescseq.siz *= 2;
			strescseq.buf = xrealloc(strescseq.buf, strescseq.siz);
		}

		memmove(&strescseq.buf[strescseq.len], c, len);
		strescseq.len += len;
		return;
	}

check_control_code:
	/*
	 * Actions of control codes must be performed as soon they arrive
	 * because they can be embedded inside a control sequence, and
	 * they must not cause conflicts with sequences.
	 */
	if (control) {
		tcontrolcode(t, u);
		/*
		 * control codes are not shown ever
		 */
		if (!t->esc)
			t->lastc = 0;
		return;
	} else if (t->esc & ESC_START) {
		if (t->esc & ESC_CSI) {
			csiescseq.buf[csiescseq.len++] = u;
			if (BETWEEN(u, 0x40, 0x7E)
					|| csiescseq.len >= \
					sizeof(csiescseq.buf)-1) {
				t->esc = 0;
				csiparse();
				csihandle(t);
			}
			return;
		} else if (t->esc & ESC_UTF8) {
			tdefutf8(t, u);
		} else if (t->esc & ESC_ALTCHARSET) {
			tdeftran(t, u);
		} else if (t->esc & ESC_TEST) {
			tdectest(t, u);
		} else {
			if (!eschandle(t, u))
				return;
			/* sequence already finished */
		}
		t->esc = 0;
		/*
		 * All characters which form part of a sequence are not
		 * printed
		 */
		return;
	}
	if (selected(t->c.x, t->c.y))
		selclear();

	gp = &t->line[t->c.y][t->c.x];
	if (IS_SET(MODE_WRAP) && (t->c.state & CURSOR_WRAPNEXT)) {
		gp->mode |= ATTR_WRAP;
		tnewline(t, 1);
		gp = &t->line[t->c.y][t->c.x];
	}

	if (IS_SET(MODE_INSERT) && t->c.x+width < t->col)
		memmove(gp+width, gp, (t->col - t->c.x - width) * sizeof(Glyph));

	if (t->c.x+width > t->col) {
		tnewline(t, 1);
		gp = &t->line[t->c.y][t->c.x];
	}

	tsetchar(t, u, &t->c.attr, t->c.x, t->c.y);
	t->lastc = u;

	if (width == 2) {
		gp->mode |= ATTR_WIDE;
		if (t->c.x+1 < t->col) {
			gp[1].u = '\0';
			gp[1].mode = ATTR_WDUMMY;
		}
	}
	if (t->c.x+width < t->col) {
		tmoveto(t, t->c.x+width, t->c.y);
	} else {
		t->c.state |= CURSOR_WRAPNEXT;
	}
}

void
tresize(Term *t, int col, int row)
{
	int i=0;
	int minrow = MIN(row, t->row);
	int mincol = MIN(col, t->col);
	int *bp;
	TCursor c;


	if (col < 1 || row < 1) {
		fprintf(stderr,
		        "tresize: error resizing to %dx%d\n", col, row);
		return;
	}

	/*
	 * slide screen to keep cursor where we expect it -
	 * tscrollup would work here, but we can optimize to
	 * memmove because we're freeing the earlier lines
	 */
    if (t->line != NULL) {
        for (i = 0; i <= t->c.y - row; i++) {
            free(t->line[i]);
            free(t->alt[i]);
        }
        /* ensure that both src and dst are not NULL */
        if (i > 0) {
            memmove(t->line, t->line + i, row * sizeof(Line));
            memmove(t->alt, t->alt + i, row * sizeof(Line));
        }
        for (i += row; i < t->row; i++) {
            free(t->line[i]);
            free(t->alt[i]);
        }
    }

	/* resize to new height */

	t->line = xrealloc(t->line, row * sizeof(Line));
	t->alt  = xrealloc(t->alt,  row * sizeof(Line));
	t->dirty = xrealloc(t->dirty, row * sizeof(*t->dirty));
	t->tabs = xrealloc(t->tabs, col * sizeof(*t->tabs));

	/* resize each row to new width, zero-pad if needed */
	for (i = 0; i < minrow; i++) {
		t->line[i] = xrealloc(t->line[i], col * sizeof(Glyph));
		t->alt[i]  = xrealloc(t->alt[i],  col * sizeof(Glyph));
	}

	/* allocate any new rows */
	for (/* i = minrow */; i < row; i++) {
		t->line[i] = xmalloc(col * sizeof(Glyph));
		t->alt[i] = xmalloc(col * sizeof(Glyph));
	}
	if (col > t->col) {
		bp = t->tabs + t->col;

		memset(bp, 0, sizeof(*t->tabs) * (col - t->col));
		while (--bp > t->tabs && !*bp)
			/* nothing */ ;
		for (bp += tabspaces; bp < t->tabs + col; bp += tabspaces)
			*bp = 1;
	}
	/* update terminal size */
	t->col = col;
	t->row = row;
	/* reset scrolling region */
	tsetscroll(t, 0, row-1);
	/* make use of the LIMIT in tmoveto */
	tmoveto(t, t->c.x, t->c.y);
	/* Clearing both screens (it makes dirty all lines) */
	c = t->c;
	for (i = 0; i < 2; i++) {
		if (mincol < col && 0 < minrow) {
			tclearregion(t, mincol, 0, col - 1, minrow - 1);
		}
		if (0 < col && minrow < row) {
			tclearregion(t, 0, minrow, col - 1, row - 1);
		}
		tswapscreen(t);
		tcursor(t, CURSOR_LOAD);
	}
	t->c = c;
}

void
resettitle(void)
{
	xsettitle(NULL);
}
