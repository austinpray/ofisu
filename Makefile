OFFICE_GRAPHS = $(shell find deploy/offices/ -type f -name '*.dot')
OFFICE_GRAPHS_IMG = $(OFFICE_GRAPHS:.dot=.png)

.PHONY: default
default: $(OFFICE_GRAPHS_IMG)

deploy/offices/%.png: deploy/offices/%.dot
	docker-compose run --rm --no-deps ofisu dot $< -Tpng -o $@
