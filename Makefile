aerc:
	go build -o aerc

%.1: doc/%.1.scd
	scdoc < $< > $@

%.5: doc/%.5.scd
	scdoc < $< > $@

DOCS := \
	aerc.1 \
	aerc-config.5 \
	aerc-imap.5 \
	aerc-smtp.5

all: aerc $(DOCS)

clean:
	rm -f *.1 *.5 aerc

PREFIX?=/usr/local
_INSTDIR=$(DESTDIR)$(PREFIX)
BINDIR?=$(_INSTDIR)/bin
SHAREDIR?=$(_INSTDIR)/share/aerc
MANDIR?=$(_INSTDIR)/share/man

install: all
	mkdir -p $(BINDIR) $(MANDIR)/man1 $(MANDIR)/man5 \
		$(SHAREDIR) $(SHAREDIR)/filters
	install -m755 aerc $(BINDIR)/aerc
	install -m644 aerc.1 $(MANDIR)/man1/aerc.1
	install -m644 aerc-config.5 $(MANDIR)/man5/aerc-config.5
	install -m644 aerc-imap.5 $(MANDIR)/man5/aerc-imap.5
	install -m644 aerc-smtp.5 $(MANDIR)/man5/aerc-smtp.5
	install -m644 config/accounts.conf $(SHAREDIR)/accounts.conf
	install -m644 config/aerc.conf $(SHAREDIR)/aerc.conf
	install -m644 config/binds.conf $(SHAREDIR)/binds.conf
	install -m755 contrib/hldiff.py $(SHAREDIR)/filters/hldiff.py
	install -m755 contrib/html $(SHAREDIR)/filters/html
	install -m755 contrib/plaintext.py $(SHAREDIR)/filters/plaintext.py

.DEFAULT_GOAL := all

.PHONY: aerc clean install