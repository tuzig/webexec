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

enum win_mode {
	MODE_VISIBLE     = 1 << 0,
	MODE_FOCUSED     = 1 << 1,
	MODE_APPKEYPAD   = 1 << 2,
	MODE_MOUSEBTN    = 1 << 3,
	MODE_MOUSEMOTION = 1 << 4,
	MODE_REVERSE     = 1 << 5,
	MODE_KBDLOCK     = 1 << 6,
	MODE_HIDE        = 1 << 7,
	MODE_APPCURSOR   = 1 << 8,
	MODE_MOUSESGR    = 1 << 9,
	MODE_8BIT        = 1 << 10,
	MODE_BLINK       = 1 << 11,
	MODE_FBLINK      = 1 << 12,
	MODE_FOCUS       = 1 << 13,
	MODE_MOUSEX10    = 1 << 14,
	MODE_MOUSEMANY   = 1 << 15,
	MODE_BRCKTPASTE  = 1 << 16,
	MODE_NUMLOCK     = 1 << 17,
	MODE_MOUSE       = MODE_MOUSEBTN|MODE_MOUSEMOTION|MODE_MOUSEX10\
	                  |MODE_MOUSEMANY,
};

void xbell(void);
void xclipcopy(void);
void xdrawcursor(int, int, Glyph, int, int, Glyph);
void xdrawline(Line, int, int, int);
void xfinishdraw(void);
void xloadcols(void);
int xsetcolorname(int, const char *);
void xsettitle(char *);
int xsetcursor(int);
void xsetmode(int, unsigned int);
void xsetpointermotion(int);
void xsetsel(char *);
int xstartdraw(void);
void xximspot(int, int);
