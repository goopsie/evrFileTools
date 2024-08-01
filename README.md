tool i ~~not so~~ quickly threw together to modify any file(s) in an EVR manifest/package combo.

thank you Exhibitmark for doing the hard work and making [carnation](https://github.com/Exhibitmark/carnation), saved me a lot of headache reversing the manifest format :)



extracting files example:
```
evrFileTools -mode extract -packageName 48037dc70b0ecab2 -dataDir ./ -outputDir ./output/
```
this will extract and write out every file contained in the package to outputFolder.
the names of the subfolders created in outputFolder are the filetype symbols, the files contained within are named with their respective symbols.

If the `-outputPreserveGroups` flag is provided, there will be folders created to seperate each frame. This is currently the directory structure that `-mode build` expects.


replacing files example:
```
echoFileTools -mode replace -outputDir ./output/ -packageName 48037dc70b0ecab2 -dataDir ./ -inputDir ./input/
```
Directory structure of inputDir should be `./inputFolder/0/...`, where ... is the structure of `-mode extract` *without* the `-outputPreserveGroups` flag.

e.g. if replacing the Echo VR logo DDS, the stucture would be as follows: `./input/0/-4707359568332879775/-3482028914369150717`


if a file with the same filetype symbol & filename symbol exists in the manifest, it will edit the manifest & package file to match, and write out the contents of both to outputFolder.
