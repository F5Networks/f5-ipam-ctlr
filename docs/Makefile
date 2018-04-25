# Minimal makefile for Sphinx documentation
#

# You can set these variables from the command line.
SPHINXOPTS    =
SPHINXBUILD   = sphinx-build
SPHINXPROJ    = F5IPAMController
SOURCEDIR     = .
BUILDDIR      = _build
ALLSPHINXOPTS   =
ifdef DOCS_RELEASE
  ifdef DOCS_VERSION
    ALLSPHINXOPTS += -D release=$(DOCS_RELEASE) -D version=$(DOCS_VERSION)
  endif
endif
ALLSPHINXOPTS   += $(SPHINXOPTS)

# Put it first so that "make" without argument is like "make help".
help:
	@$(SPHINXBUILD) -M help "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O)

.PHONY: help Makefile

# Catch-all target: route all unknown targets to Sphinx using the new
# "make mode" option.  $(O) is meant as a shortcut for $(SPHINXOPTS).
%: Makefile
	@$(SPHINXBUILD) -M $@ "$(SOURCEDIR)" "$(BUILDDIR)" $(ALLSPHINXOPTS) $(O)
