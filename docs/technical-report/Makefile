MAIN = main
BUILD = build

.PHONY: pdf watch clean

pdf:
	@mkdir -p $(BUILD)
	proot-distro login ubuntu -- bash -c "cd /data/data/com.termux/files/home/Doki/docs/technical-report && xelatex -interaction=nonstopmode -output-directory=$(BUILD) $(MAIN).tex"
	cp references.bib $(BUILD)/
	proot-distro login ubuntu -- bash -c "cd /data/data/com.termux/files/home/Doki/docs/technical-report/$(BUILD) && biber $(MAIN)"
	proot-distro login ubuntu -- bash -c "cd /data/data/com.termux/files/home/Doki/docs/technical-report && xelatex -interaction=nonstopmode -output-directory=$(BUILD) $(MAIN).tex"
	proot-distro login ubuntu -- bash -c "cd /data/data/com.termux/files/home/Doki/docs/technical-report && xelatex -interaction=nonstopmode -output-directory=$(BUILD) $(MAIN).tex"

clean:
	rm -rf $(BUILD)
