NAME := "loadbars"
VERSION := "0.7.5"

default: version documentation perltidy

version:
    echo {{VERSION}} > .version

profile:
    perl -d:NYTProf loadbars --hosts localhost
    nytprofhtml nytprof.out

perltidy:
    find . -name \*.pm | xargs perltidy -b
    perltidy -b {{NAME}}
    find . -name \*.bak -delete

documentation:
    pandoc --standalone --to man ./README.md --metadata title="loadbars" --metadata section="1" --metadata date="$(date +%Y-%m-%d)" > ./docs/{{NAME}}.1
    pandoc --from markdown --to plain ./README.md > ./docs/{{NAME}}.txt

install DESTDIR="":
    #!/usr/bin/env bash
    if [ ! -d "{{DESTDIR}}/usr/bin" ]; then
        mkdir -p {{DESTDIR}}/usr/bin
    fi
    if [ ! -d "{{DESTDIR}}/usr/share/{{NAME}}" ]; then
        mkdir -p {{DESTDIR}}/usr/share/{{NAME}}
    fi
    cp {{NAME}} {{DESTDIR}}/usr/bin
    cp -r ./lib {{DESTDIR}}/usr/share/{{NAME}}/lib
    cp -r ./fonts {{DESTDIR}}/usr/share/{{NAME}}/fonts
    cp ./.version {{DESTDIR}}/usr/share/{{NAME}}/version

deinstall DESTDIR="":
    #!/usr/bin/env bash
    if [ -n "{{DESTDIR}}" ] && [ -f "{{DESTDIR}}/usr/bin/{{NAME}}" ]; then
        rm {{DESTDIR}}/usr/bin/{{NAME}}
    fi
    if [ -n "{{DESTDIR}}" ] && [ -d "{{DESTDIR}}/usr/share/{{NAME}}" ]; then
        rm -r {{DESTDIR}}/usr/share/{{NAME}}
    fi

clean:
    #!/usr/bin/env bash
    if [ -f nytprof.out ]; then
        rm nytprof.out
    fi
    if [ -f tmon.out ]; then
        rm tmon.out
    fi
    if [ -d nytprof ]; then
        rm -Rf nytprof
    fi

release: version documentation perltidy
    git add -A
    git commit -m 'New release {{VERSION}}'
    git tag {{VERSION}}
    git push --tags
    git push origin master
