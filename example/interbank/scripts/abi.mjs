import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/contracts/shared/Atom.sol/AtomFactory.json",
  "src/abis/AtomFactory.json"
);

await copyFile(
  "../../solidity/artifacts/contracts/shared/Atom.sol/Atom.json",
  "src/abis/Atom.json"
);
