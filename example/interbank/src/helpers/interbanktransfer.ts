import PaladinClient, {
  PaladinVerifier,
  PentePrivacyGroup,
  PentePrivateContract,
} from "@lfdecentralizedtrust-labs/paladin-sdk";
import interbankTransfer from "../abis/InterbankTransfer.json";

const constructorAbi = interbankTransfer.abi.find(
  (entry) => entry.type === "constructor"
);

export interface Participant {
  token: string;
  wallet: string;
  cbdcWallet: string;
}

export interface InterbankTransferConstructorParams {
  cbdc_: string;
  sender_: Participant;
  receiver_: Participant;
  atomFactory_: string;
}

export interface StateEncoded {
  id: string;
  domain: string;
  schema: string;
  contractAddress: string;
  data: string;
}

export interface StateData {
  inputs: StateEncoded[];
  outputs: StateEncoded[];
}

export interface StartTransferParams {
  value: string;
  senderStates: StateData;
  senderCall: string;
  cbdcStates: StateData;
  cbdcCall: string;
}

export interface AcceptTransferParams {
  index: string;
  receiverStates: StateData;
  receiverCall: string;
}

export const newInterbankTransfer = async (
  pente: PentePrivacyGroup,
  from: PaladinVerifier,
  params: InterbankTransferConstructorParams
) => {
  if (constructorAbi === undefined) {
    throw new Error("Bond subscription constructor not found");
  }
  const address = await pente
    .deploy({
      abi: interbankTransfer.abi,
      bytecode: interbankTransfer.bytecode,
      from: from.lookup,
      inputs: params,
    })
    .waitForDeploy();
  return address ? new InterbankTransfer(pente, address) : undefined;
};

export class InterbankTransfer extends PentePrivateContract<InterbankTransferConstructorParams> {
  constructor(
    protected evm: PentePrivacyGroup,
    public readonly address: string
  ) {
    super(evm, interbankTransfer.abi, address);
  }

  using(paladin: PaladinClient) {
    return new InterbankTransfer(this.evm.using(paladin), this.address);
  }

  startTransfer(from: PaladinVerifier, params: StartTransferParams) {
    return this.sendTransaction({
      from: from.lookup,
      function: "startTransfer",
      data: params,
    });
  }

  acceptTransfer(from: PaladinVerifier, params: AcceptTransferParams) {
    return this.sendTransaction({
      from: from.lookup,
      function: "acceptTransfer",
      data: params,
    });
  }
}
